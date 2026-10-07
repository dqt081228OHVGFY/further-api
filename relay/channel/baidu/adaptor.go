package baidu

import (
	"context"
	"errors"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	requestContext context.Context
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("baidu", "Gemini content input")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, openai.UnsupportedOperation("baidu", "Anthropic Messages input")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("baidu", "audio operations")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation("baidu", "image generation")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.requestContext = nil
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Baidu provider configuration is missing")
	}
	model := info.UpstreamModelName
	routes := map[string]string{"ERNIE-4.0": "completions_pro", "ERNIE-Bot-4": "completions_pro", "ERNIE-Bot": "completions", "ERNIE-Bot-turbo": "eb-instant", "ERNIE-Speed": "ernie_speed", "ERNIE-4.0-8K": "completions_pro", "ERNIE-3.5-8K": "completions", "ERNIE-3.5-8K-0205": "ernie-3.5-8k-0205", "ERNIE-3.5-8K-1222": "ernie-3.5-8k-1222", "ERNIE-Bot-8K": "ernie_bot_8k", "ERNIE-3.5-4K-0205": "ernie-3.5-4k-0205", "ERNIE-Speed-8K": "ernie_speed", "ERNIE-Speed-128K": "ernie-speed-128k", "ERNIE-Lite-8K-0922": "eb-instant", "ERNIE-Lite-8K-0308": "ernie-lite-8k", "ERNIE-Tiny-8K": "ernie-tiny-8k", "BLOOMZ-7B": "bloomz_7b1", "Embedding-V1": "embedding-v1", "bge-large-zh": "bge_large_zh", "bge-large-en": "bge_large_en", "tao-8k": "tao_8k"}
	task := "chat/"
	if strings.HasPrefix(model, "Embedding") || strings.HasPrefix(model, "bge-large") || strings.HasPrefix(model, "tao-8k") {
		task = "embeddings/"
	}
	deployment := routes[model]
	if deployment == "" {
		deployment = strings.ToLower(model)
	}
	target, err := openai.ProviderEndpoint(info.ChannelBaseUrl, "/rpc/2.0/ai_custom/v1/wenxinworkshop/"+task+url.PathEscape(deployment))
	if err != nil {
		return "", err
	}
	token, err := getBaiduAccessTokenContext(a.requestContext, info.ApiKey)
	if err != nil {
		return "", err
	}
	endpoint, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || headers == nil || info == nil || !info.HasChannelMeta() {
		return errors.New("Baidu request configuration is missing")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	channel.SetupApiRequestHeader(info, c, headers)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	default:
		baiduRequest := requestOpenAI2Baidu(*request)
		return baiduRequest, nil
	}
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("baidu", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	baiduEmbeddingRequest := embeddingRequestOpenAI2Baidu(request)
	return baiduEmbeddingRequest, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("baidu", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("Baidu request context is missing")
	}
	a.requestContext = c.Request.Context()
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	if info.IsStream {
		err, usage = baiduStreamHandler(c, info, resp)
	} else {
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			err, usage = baiduEmbeddingHandler(c, info, resp)
		default:
			err, usage = baiduHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
