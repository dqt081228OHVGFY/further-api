package dify

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func uploadDifyFile(c *gin.Context, info *relaycommon.RelayInfo, user string, media dto.MediaContent) (*DifyFile, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Dify upload context is missing")
	}
	source := media.ToFileSource()
	if source == nil {
		return nil, errors.New("Dify image source is missing")
	}
	encoded, contentType, err := service.GetBase64Data(c, source, "Dify inline image")
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	endpoint, err := openai.ProviderEndpoint(info.ChannelBaseUrl, "/v1/files/upload")
	if err != nil {
		return nil, err
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err = writer.WriteField("user", user); err != nil {
		return nil, err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": "image." + strings.TrimPrefix(contentType, "image/")}))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(data); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+info.ApiKey)
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Dify image upload failed (HTTP %d)", response.StatusCode)
	}
	var wire struct {
		ID string `json:"id"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&wire); err != nil {
		return nil, err
	}
	if wire.ID == "" {
		return nil, errors.New("Dify image upload returned no file ID")
	}
	return &DifyFile{Type: "image", TransferMode: "local_file", UploadFileId: wire.ID}, nil
}
func requestOpenAI2Dify(c *gin.Context, info *relaycommon.RelayInfo, request dto.GeneralOpenAIRequest) (*DifyChatRequest, error) {
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Dify provider configuration is missing")
	}
	user := ""
	if len(request.User) > 0 {
		_ = json.Unmarshal(request.User, &user)
	}
	if user == "" {
		user = helper.GetResponseID(c)
	}
	output := &DifyChatRequest{Inputs: make(map[string]any), User: user, Files: make([]DifyFile, 0), ResponseMode: "blocking"}
	if request.Stream != nil && *request.Stream {
		output.ResponseMode = "streaming"
	}
	var text strings.Builder
	for _, message := range request.Messages {
		role := strings.ToUpper(message.Role)
		if role == "DEVELOPER" {
			role = "SYSTEM"
		}
		for _, part := range message.ParseContent() {
			switch part.Type {
			case dto.ContentTypeText:
				text.WriteString(role + ": \n" + part.Text + "\n")
			case dto.ContentTypeImageURL:
				media := part.GetImageMedia()
				if media == nil || media.Url == "" {
					return nil, errors.New("Dify image URL is missing")
				}
				if media.IsRemoteImage() {
					output.Files = append(output.Files, DifyFile{Type: "image", TransferMode: "remote_url", URL: media.Url})
				} else {
					file, err := uploadDifyFile(c, info, user, part)
					if err != nil {
						return nil, err
					}
					output.Files = append(output.Files, *file)
				}
			default:
				return nil, openai.UnsupportedOperation("Dify", "this message content type")
			}
		}
	}
	output.Query = text.String()
	return output, nil
}
func streamResponseDify2OpenAI(wire DifyChunkChatCompletionResponse) *dto.ChatCompletionsStreamResponse {
	frame := &dto.ChatCompletionsStreamResponse{Id: wire.ConversationId, Object: "chat.completion.chunk", Created: common.GetTimestamp(), Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0}}}
	choice := &frame.Choices[0]
	switch wire.Event {
	case "message", "agent_message":
		answer := wire.Answer
		if strings.HasPrefix(answer, "<details") && strings.Contains(answer, "Thinking...") {
			answer = "<think>"
		} else if answer == "</details>" {
			answer = "</think>"
		}
		choice.Delta.SetContentString(answer)
	case "message_end":
		reason := "stop"
		choice.FinishReason = &reason
		frame.Usage = &wire.MetaData.Usage
	default:
		if constant.DifyDebug {
			if strings.HasPrefix(wire.Event, "workflow_") {
				choice.Delta.SetReasoningContent("Workflow: " + wire.Data.WorkflowId + " " + wire.Data.Status + "\n")
			} else if strings.HasPrefix(wire.Event, "node_") {
				choice.Delta.SetReasoningContent("Node: " + wire.Data.NodeType + " " + wire.Data.Status + "\n")
			}
		}
	}
	return frame
}
func difyStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextStream(c, info, response, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var wire *DifyChunkChatCompletionResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Dify stream frame must be an object"), types.ErrorCodeBadResponseBody)
		}
		if wire.Event == "error" {
			var detail struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &detail)
			if detail.Message == "" {
				detail.Message = "Dify stream failed"
			}
			return nil, types.NewError(errors.New(detail.Message), types.ErrorCodeBadResponseBody)
		}
		frame := streamResponseDify2OpenAI(*wire)
		frame.Model = info.UpstreamModelName
		return frame, nil
	})
}
func difyHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *DifyChatCompletionResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Dify response must be an object"), types.ErrorCodeBadResponseBody)
		}
		return &dto.OpenAITextResponse{Id: wire.ConversationId, Object: "chat.completion", Created: wire.CreateAt, Model: info.UpstreamModelName, Usage: wire.MetaData.Usage, Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: wire.Answer}, FinishReason: "stop"}}}, nil
	})
}
