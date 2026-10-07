package siliconflow

import (
	"errors"

	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct{ ResponseFormat string }

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	body, err := openai.AudioPayload(c, info, request)
	if err == nil {
		a.ResponseFormat = request.ResponseFormat
	}
	return body, err
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// 解析extra到SFImageRequest里，以填入SiliconFlow特殊字段。若失败重建一个空的。
	sfRequest := &SFImageRequest{}
	extra, err := common.Marshal(request.Extra)
	if err == nil {
		err = common.Unmarshal(extra, sfRequest)
		if err != nil {
			sfRequest = &SFImageRequest{}
		}
	}

	sfRequest.Model = request.Model
	sfRequest.Prompt = request.Prompt
	// 优先使用image_size/batch_size，否则使用OpenAI标准的size/n
	if sfRequest.ImageSize == "" {
		sfRequest.ImageSize = request.Size
	}
	if sfRequest.BatchSize == 0 {
		if request.N != nil {
			sfRequest.BatchSize = lo.FromPtr(request.N)
		}
	}

	return sfRequest, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.ResponseFormat = ""
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("SiliconFlow provider configuration is missing")
	}
	if info.RelayMode == constant.RelayModeRerank {
		return openai.ProviderEndpoint(info.ChannelBaseUrl, "/v1/rerank")
	}
	return openai.RequestEndpoint(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	return openai.SetupProviderHeaders(c, headers, info)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("SiliconFlow chat request is missing")
	}
	// SiliconFlow requires messages array for FIM requests, even if client doesn't send it
	if (request.Prefix != nil || request.Suffix != nil) && len(request.Messages) == 0 {
		// Add an empty user message to satisfy SiliconFlow's requirement
		request.Messages = []dto.Message{
			{
				Role:    "user",
				Content: "",
			},
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("siliconflow", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if info == nil {
		return nil, errors.New("SiliconFlow request context is missing")
	}
	switch info.RelayMode {
	case constant.RelayModeAudioTranscription, constant.RelayModeAudioTranslation:
		return channel.DoFormRequest(a, c, info, body)
	case constant.RelayModeRealtime:
		return channel.DoWssRequest(a, c, info, body)
	default:
		return channel.DoApiRequest(a, c, info, body)
	}
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	switch info.RelayMode {
	case constant.RelayModeRerank:
		usage, err = siliconflowRerankHandler(c, info, resp)
	default:
		usage, err = openai.ProtocolResponse(c, resp, info, a.ResponseFormat)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
