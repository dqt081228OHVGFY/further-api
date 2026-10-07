package palm

import (
	"errors"
	"net/url"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "Gemini content input")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "Anthropic Messages input")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("palm", "audio operations")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "image generation")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("PaLM provider configuration is missing")
	}
	model := info.UpstreamModelName
	if model == "" {
		model = "chat-bison-001"
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, "/v1beta2/models/"+url.PathEscape(model)+":generateMessage")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("PaLM request is missing")
	}
	output := &PaLMChatRequest{Temperature: request.Temperature, Prompt: PaLMPrompt{Messages: make([]PaLMChatMessage, 0, len(request.Messages))}}
	for _, message := range request.Messages {
		if message.Role == "system" || message.Role == "developer" {
			if output.Prompt.Context != "" {
				output.Prompt.Context += "\n"
			}
			output.Prompt.Context += message.StringContent()
		} else {
			output.Prompt.Messages = append(output.Prompt.Messages, PaLMChatMessage{Author: message.Role, Content: message.StringContent()})
		}
	}
	if request.N != nil {
		output.CandidateCount = *request.N
	}
	if request.TopP != nil {
		output.TopP = *request.TopP
	}
	if request.TopK != nil && *request.TopK >= 0 {
		output.TopK = uint(*request.TopK)
	}
	return output, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "embeddings")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("palm", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	if info.IsStream {
		var responseText string
		err, responseText = palmStreamHandler(c, info, resp)
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	} else {
		usage, err = palmHandler(c, info, resp)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
