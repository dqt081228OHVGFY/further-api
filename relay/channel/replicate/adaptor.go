package replicate

import (
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type Adaptor struct{}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("Replicate 通道信息缺失")
	}
	return replicateAPIURL(info, info.RequestURLPath)
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil || info.ApiKey == "" {
		return errors.New("Replicate 密钥尚未配置")
	}
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Prefer", "wait")
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	header.Set("Accept", "application/json")
	return nil
}
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, input dto.ImageRequest) (any, error) {
	return prepareReplicateImage(c, info, input)
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, body)
}
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	return finishReplicateImage(c, response, info)
}
func (a *Adaptor) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持聊天请求")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持重排请求")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持嵌入请求")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("Replicate 图片通道不支持音频请求")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持 Responses 请求")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持 Claude 聊天请求")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("Replicate 图片通道不支持 Gemini 聊天请求")
}
