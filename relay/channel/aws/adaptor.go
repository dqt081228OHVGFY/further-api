package aws

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type ClientMode int

const (
	ClientModeApiKey ClientMode = iota + 1
	ClientModeAKSK
)

type Adaptor struct {
	ClientMode ClientMode
	AwsClient  *bedrockruntime.Client
	AwsModelId string
	AwsReq     any
	IsNova     bool
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	*a = Adaptor{ClientMode: ClientModeAKSK}
	if info != nil && info.HasChannelMeta() {
		if identity, err := bedrockIdentityFrom(info.ApiKey); err == nil && identity.bearerToken != "" {
			a.ClientMode = ClientModeApiKey
		}
	}
}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return bedrockInvokeEndpoint(info)
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil || !info.HasChannelMeta() || headers == nil {
		return errors.New("Bedrock headers require provider configuration")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	claude.CommonClaudeHeadersOperation(c, headers, info)
	identity, err := bedrockIdentityFrom(info.ApiKey)
	if err != nil {
		return err
	}
	if identity.bearerToken != "" {
		headers.Set("Authorization", "Bearer "+identity.bearerToken)
	}
	return nil
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if info != nil && info.HasChannelMeta() && isNovaModel(getAwsModelID(info.UpstreamModelName)) {
		chat, err := openai.ChatFrom(c, info, request)
		if err != nil {
			return nil, err
		}
		return a.ConvertOpenAIRequest(c, info, chat)
	}
	native, err := claude.NativeMessages(c, info, request)
	if err != nil {
		return nil, err
	}
	// Bedrock Messages consumes inline media. Fetches use the gateway's file
	// service so retrying does not bypass its request policy or media cache.
	for index := range native.Messages {
		message := &native.Messages[index]
		if message.IsStringContent() {
			continue
		}
		blocks, err := message.ParseContent()
		if err != nil {
			return nil, err
		}
		changed := false
		for i := range blocks {
			source := blocks[i].Source
			if source == nil || source.Type != "url" {
				continue
			}
			data, mediaType, err := service.GetBase64Data(c, types.NewURLFileSource(source.Url), "Bedrock inline media")
			if err != nil {
				return nil, fmt.Errorf("prepare Bedrock media: %w", err)
			}
			source.Type, source.MediaType, source.Data, source.Url = "base64", mediaType, data, ""
			changed = true
		}
		if changed {
			message.SetContent(blocks)
		}
	}
	return native, nil
}
func (a *Adaptor) bedrockMessages(c *gin.Context, info *relaycommon.RelayInfo, source any) (any, error) {
	request, err := claude.MessagesFrom(c, info, source)
	if err != nil {
		return nil, err
	}
	return a.ConvertClaudeRequest(c, info, request)
}
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("Bedrock chat request is missing")
	}
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Bedrock provider configuration is missing")
	}
	if isNovaModel(getAwsModelID(info.UpstreamModelName)) {
		converted, err := novaRequest(c, request)
		if err == nil {
			a.IsNova = true
		}
		return converted, err
	}
	return a.bedrockMessages(c, info, request)
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if request == nil {
		return nil, errors.New("Gemini request is missing")
	}
	if info != nil && info.HasChannelMeta() && isNovaModel(getAwsModelID(info.UpstreamModelName)) {
		chat, err := openai.ChatFrom(c, info, request)
		if err != nil {
			return nil, err
		}
		return a.ConvertOpenAIRequest(c, info, chat)
	}
	return a.bedrockMessages(c, info, request)
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if info != nil && info.HasChannelMeta() && isNovaModel(getAwsModelID(info.UpstreamModelName)) {
		converted, err := openai.ChatFrom(c, info, &request)
		if err != nil {
			return nil, err
		}
		return a.ConvertOpenAIRequest(c, info, converted)
	}
	return a.bedrockMessages(c, info, &request)
}
func unsupportedBedrock(operation string) error {
	return openai.UnsupportedOperation("this Bedrock model", operation)
}
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, unsupportedBedrock("audio operations")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, unsupportedBedrock("image generation")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, unsupportedBedrock("reranking")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, unsupportedBedrock("embeddings")
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	return doAwsClientRequest(c, info, a, body)
}
func (a *Adaptor) DoResponse(c *gin.Context, _ *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if info == nil {
		return nil, invalidAwsOperation(errors.New("Bedrock response context is missing"))
	}
	var failure *types.GatewayError
	var usage *dto.Usage
	if a.IsNova {
		failure, usage = handleNovaRequest(c, info, a)
	} else if info.IsStream && info.RelayFormat == types.RelayFormatOpenAIResponses {
		failure, usage = awsResponsesStream(c, info, a)
	} else if info.IsStream {
		failure, usage = awsStreamHandler(c, info, a)
	} else {
		failure, usage = awsHandler(c, info, a)
	}
	return usage, failure
}
func (a *Adaptor) GetModelList() []string {
	models := make([]string, 0, len(awsModelIDMap))
	for model := range awsModelIDMap {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
func (a *Adaptor) GetChannelName() string { return ChannelName }
