package minimax

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("minimax", "Gemini content input")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return claude.NativeMessages(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() || info.RelayMode != constant.RelayModeAudioSpeech {
		return nil, errors.New("unsupported audio relay mode")
	}
	if err := c.Request.Context().Err(); err != nil {
		return nil, err
	}
	format := request.ResponseFormat
	if format == "" || format == "hex" || format == "url" {
		format = "mp3"
	}
	input := MiniMaxTTSRequest{Text: request.Input, VoiceSetting: VoiceSetting{VoiceID: request.Voice, Speed: lo.FromPtrOr(request.Speed, 1)}, AudioSetting: &AudioSetting{Format: format}, OutputFormat: "hex"}
	if len(request.Metadata) > 0 {
		if err := json.Unmarshal(request.Metadata, &input); err != nil {
			return nil, fmt.Errorf("MiniMax speech options are invalid: %w", err)
		}
	}
	input.Model = info.UpstreamModelName
	if input.Model == "" {
		input.Model = request.Model
	}
	input.Stream = info.IsStream
	if input.Stream {
		input.OutputFormat = "hex"
		input.StreamOptions = &StreamOptions{ExcludeAggregatedAudio: true}
	}
	if input.AudioSetting == nil {
		input.AudioSetting = &AudioSetting{Format: format}
	}
	if input.AudioSetting.Format == "" {
		input.AudioSetting.Format = format
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	c.Set("response_format", input.AudioSetting.Format)
	c.Set("minimax_stream_format", request.StreamFormat)
	return bytes.NewReader(data), nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode != constant.RelayModeImagesGenerations {
		return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
	}
	return oaiImage2MiniMaxImageRequest(request), nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return GetRequestURL(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("minimax", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("minimax", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	if info.RelayMode == constant.RelayModeAudioSpeech {
		return handleTTSResponse(c, resp, info)
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		return miniMaxImageHandler(c, resp, info)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return claude.MessagesResponse(c, resp, info)
	default:
		return openai.ProtocolResponse(c, resp, info, "")
	}
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
