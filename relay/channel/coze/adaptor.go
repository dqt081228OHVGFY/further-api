package coze

import (
	"context"
	"encoding/json"
	"errors"
	gatewaycommon "github.com/dqt081228OHVGFY/further-api/common"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"io"
	"net/http"
	"time"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *common.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "Gemini content input")
}

// ConvertAudioRequest implements channel.Adaptor.
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *common.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("coze", "audio operations")
}

// ConvertClaudeRequest implements channel.Adaptor.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *common.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "Anthropic Messages input")
}

// ConvertEmbeddingRequest implements channel.Adaptor.
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *common.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "embeddings")
}

// ConvertImageRequest implements channel.Adaptor.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *common.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "image generation")
}

// ConvertOpenAIRequest implements channel.Adaptor.
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *common.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return convertCozeChatRequest(c, *request), nil
}

// ConvertOpenAIResponsesRequest implements channel.Adaptor.
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *common.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "Responses input")
}

// ConvertRerankRequest implements channel.Adaptor.
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("coze", "reranking")
}

// DoRequest implements channel.Adaptor.
func (a *Adaptor) DoRequest(c *gin.Context, info *common.RelayInfo, body io.Reader) (any, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Coze request context is missing")
	}
	if info.IsStream {
		return channel.DoApiRequest(a, c, info, body)
	}
	response, err := channel.DoApiRequest(a, c, info, body)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("Coze returned no creation response")
	}
	var created *CozeChatResponse
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&created)
	response.Body.Close()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if created == nil {
		return nil, errors.New("Coze creation response must be an object")
	}
	if created.Code != 0 {
		return nil, errors.New(created.Msg)
	}
	if created.Data.ConversationId == "" || created.Data.Id == "" {
		return nil, errors.New("Coze creation response is missing conversation or chat ID")
	}
	c.Set("coze_conversation_id", created.Data.ConversationId)
	c.Set("coze_chat_id", created.Data.Id)
	timeout := 10 * time.Minute
	if gatewaycommon.RelayTimeout > 0 {
		timeout = time.Duration(gatewaycommon.RelayTimeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	original := c.Request
	c.Request = original.WithContext(ctx)
	defer func() { c.Request = original }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		err, done := checkIfChatComplete(a, c, info)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	return getChatDetail(a, c, info)
}

// DoResponse implements channel.Adaptor.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *common.RelayInfo) (usage any, err *types.GatewayError) {
	if info.IsStream {
		usage, err = cozeChatStreamHandler(c, info, resp)
	} else {
		usage, err = cozeChatHandler(c, info, resp)
	}
	return
}

// GetChannelName implements channel.Adaptor.
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// GetModelList implements channel.Adaptor.
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetRequestURL implements channel.Adaptor.
func (a *Adaptor) GetRequestURL(info *common.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Coze provider configuration is missing")
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, "/v3/chat")
}

// Init implements channel.Adaptor.
func (a *Adaptor) Init(info *common.RelayInfo) {

}

// SetupRequestHeader implements channel.Adaptor.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *common.RelayInfo) error {
	return openai.SetupProviderHeaders(c, headers, info)
}
