package coze

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func convertCozeChatRequest(c *gin.Context, request dto.GeneralOpenAIRequest) *CozeChatRequest {
	user := request.User
	var identity string
	if json.Unmarshal(user, &identity) != nil || identity == "" {
		user, _ = json.Marshal(helper.GetResponseID(c))
	}
	output := &CozeChatRequest{BotId: c.GetString("bot_id"), UserId: user, Stream: request.Stream != nil && *request.Stream, AdditionalMessages: make([]CozeEnterMessage, 0, len(request.Messages))}
	for _, message := range request.Messages {
		role := message.Role
		content := message.StringContent()
		if role != "user" && role != "assistant" {
			content = strings.ToUpper(role) + ":\n" + content
			role = "user"
		}
		output.AdditionalMessages = append(output.AdditionalMessages, CozeEnterMessage{Role: role, Content: content, ContentType: "text"})
	}
	return output
}
func cozeChatHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *CozeChatDetailResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Coze detail response must be an object"), types.ErrorCodeBadResponseBody)
		}
		if wire.Code != 0 {
			return nil, types.NewError(errors.New(wire.Msg), types.ErrorCodeBadResponseBody)
		}
		var text strings.Builder
		created := common.GetTimestamp()
		for _, message := range wire.Data {
			if message.Type == "answer" {
				var content string
				if err := json.Unmarshal(message.Content, &content); err != nil {
					return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
				}
				text.WriteString(content)
				if message.CreatedAt != 0 {
					created = message.CreatedAt
				}
			}
		}
		usage := dto.Usage{PromptTokens: c.GetInt("coze_input_count"), CompletionTokens: c.GetInt("coze_output_count"), TotalTokens: c.GetInt("coze_token_count")}
		return &dto.OpenAITextResponse{Id: helper.GetResponseID(c), Object: "chat.completion", Created: created, Model: info.UpstreamModelName, Usage: usage, Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: text.String()}, FinishReason: "stop"}}}, nil
	})
}

type cozeEvents struct {
	source  io.ReadCloser
	scanner *bufio.Scanner
	event   string
	data    []string
	pending []byte
	ended   bool
}

func (body *cozeEvents) frame() error {
	if body.event == "" || len(body.data) == 0 {
		body.event = ""
		body.data = nil
		return nil
	}
	value := strings.Join(body.data, "\n")
	if !json.Valid([]byte(value)) {
		return errors.New("Coze SSE event data is not valid JSON")
	}
	encoded, err := json.Marshal(struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{body.event, json.RawMessage(value)})
	if err != nil {
		return err
	}
	body.pending = append([]byte("data: "), encoded...)
	body.pending = append(body.pending, '\n', '\n')
	body.event = ""
	body.data = nil
	return nil
}
func (body *cozeEvents) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for len(body.pending) == 0 {
		if body.ended {
			return 0, io.EOF
		}
		if !body.scanner.Scan() {
			body.ended = true
			if err := body.scanner.Err(); err != nil {
				return 0, err
			}
			if err := body.frame(); err != nil {
				return 0, err
			}
			continue
		}
		line := strings.TrimSuffix(body.scanner.Text(), "\r")
		if line == "" {
			if err := body.frame(); err != nil {
				return 0, err
			}
		} else if event, found := strings.CutPrefix(line, "event:"); found {
			body.event = strings.TrimSpace(event)
		} else if data, found := strings.CutPrefix(line, "data:"); found {
			body.data = append(body.data, strings.TrimPrefix(data, " "))
		}
	}
	n := copy(target, body.pending)
	body.pending = body.pending[n:]
	return n, nil
}
func (body *cozeEvents) Close() error { return body.source.Close() }
func cozeChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Coze stream body is missing"), types.ErrorCodeBadResponseBody)
	}
	adapted := *response
	adapted.Body = &cozeEvents{source: response.Body, scanner: helper.NewStreamScanner(response.Body)}
	return openai.NativeTextStream(c, info, &adapted, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var event struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		frame := &dto.ChatCompletionsStreamResponse{Id: helper.GetResponseID(c), Object: "chat.completion.chunk", Created: common.GetTimestamp(), Model: info.UpstreamModelName, Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0}}}
		switch event.Event {
		case "conversation.chat.completed":
			var completed CozeChatResponseData
			if err := json.Unmarshal(event.Data, &completed); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			reason := "stop"
			frame.Choices[0].FinishReason = &reason
			frame.Usage = &dto.Usage{PromptTokens: completed.Usage.InputCount, CompletionTokens: completed.Usage.OutputCount, TotalTokens: completed.Usage.TokenCount}
		case "conversation.message.delta":
			var message CozeChatV3MessageDetail
			if err := json.Unmarshal(event.Data, &message); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			if message.Type != "answer" && message.Type != "" {
				return nil, nil
			}
			var content string
			if err := json.Unmarshal(message.Content, &content); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			frame.Choices[0].Delta.SetContentString(content)
		case "error":
			var failure CozeError
			if err := json.Unmarshal(event.Data, &failure); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			return nil, types.NewError(errors.New(failure.Message), types.ErrorCodeBadResponseBody)
		case "conversation.chat.failed":
			var failure CozeChatResponseData
			_ = json.Unmarshal(event.Data, &failure)
			return nil, types.NewError(errors.New(failure.LastError.Message), types.ErrorCodeBadResponseBody)
		default:
			return nil, nil
		}
		return frame, nil
	})
}

func cozeQuery(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo, route string) (*http.Response, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Coze query context is missing")
	}
	target, err := openai.ProviderEndpoint(info.ChannelBaseUrl, route)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("conversation_id", c.GetString("coze_conversation_id"))
	query.Set("chat_id", c.GetString("coze_chat_id"))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	if err = a.SetupRequestHeader(c, &request.Header, info); err != nil {
		return nil, err
	}
	return doRequest(request, info)
}
func checkIfChatComplete(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (error, bool) {
	response, err := cozeQuery(a, c, info, "/v3/chat/retrieve")
	if err != nil {
		return err, false
	}
	defer response.Body.Close()
	var wire *CozeChatResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&wire); err != nil {
		return err, false
	}
	if wire == nil {
		return errors.New("Coze retrieve response must be an object"), false
	}
	if wire.Code != 0 {
		return errors.New(wire.Msg), false
	}
	switch wire.Data.Status {
	case "completed":
		c.Set("coze_token_count", wire.Data.Usage.TokenCount)
		c.Set("coze_output_count", wire.Data.Usage.OutputCount)
		c.Set("coze_input_count", wire.Data.Usage.InputCount)
		return nil, true
	case "failed", "canceled", "requires_action":
		return fmt.Errorf("Coze chat status %s: %s", wire.Data.Status, wire.Data.LastError.Message), false
	default:
		return nil, false
	}
}
func getChatDetail(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (*http.Response, error) {
	return cozeQuery(a, c, info, "/v3/chat/message/list")
}
func doRequest(request *http.Request, info *relaycommon.RelayInfo) (*http.Response, error) {
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("Coze query failed (HTTP %d)", response.StatusCode)
	}
	return response, nil
}
