package openai

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func SetupProviderHeaders(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil || !info.HasChannelMeta() || headers == nil || c == nil || c.Request == nil {
		return errors.New("OpenAI authentication requires a request, headers and provider configuration")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	channel.SetupApiRequestHeader(info, c, headers)
	if ProviderFor(info.ChannelType).Azure {
		headers.Set("api-key", info.ApiKey)
		return nil
	}
	if info.ChannelType == constant.ChannelTypeOpenAI && info.Organization != "" {
		headers.Set("OpenAI-Organization", info.Organization)
	}
	overridden := false
	for name := range info.HeadersOverride {
		if strings.EqualFold(name, "Authorization") {
			overridden = true
			break
		}
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		legacy := strings.Contains(info.UpstreamModelName, "-realtime-preview")
		if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
			protocols := []string{"realtime", "openai-insecure-api-key." + info.ApiKey}
			if legacy {
				protocols = append(protocols, "openai-beta.realtime-v1")
			}
			headers.Set("Sec-WebSocket-Protocol", strings.Join(protocols, ","))
		} else {
			if legacy {
				headers.Set("OpenAI-Beta", "realtime=v1")
			}
			if !overridden {
				headers.Set("Authorization", "Bearer "+info.ApiKey)
			}
		}
	} else if !overridden {
		headers.Set("Authorization", "Bearer "+info.ApiKey)
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		defaults := map[string]string{"HTTP-Referer": "https://github.com/dqt081228OHVGFY/further-api", "X-OpenRouter-Title": "further-api"}
		for name, value := range defaults {
			if headers.Get(name) == "" {
				headers.Set(name, value)
			}
		}
	}
	return nil
}
