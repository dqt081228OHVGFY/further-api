package minimax

import (
	"errors"
	channelconstant "github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("MiniMax provider configuration is missing")
	}
	base := info.ChannelBaseUrl
	if base == "" {
		base = channelconstant.GetChannelBaseURL(channelconstant.ChannelTypeMiniMax)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		return openai.ProviderEndpoint(base, "/anthropic/v1/messages")
	}
	routes := map[int]string{constant.RelayModeChatCompletions: "/v1/text/chatcompletion_v2", constant.RelayModeImagesGenerations: "/v1/image_generation", constant.RelayModeAudioSpeech: "/v1/t2a_v2"}
	route, ok := routes[info.RelayMode]
	if !ok {
		return "", openai.UnsupportedOperation("MiniMax", "this operation")
	}
	return openai.ProviderEndpoint(base, route)
}
