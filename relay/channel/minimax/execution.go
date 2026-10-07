package minimax

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func miniMaxExecutionError(err error) *types.GatewayError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
}
func miniMaxRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}
func readMiniMaxResponse(c *gin.Context, response *http.Response) ([]byte, error) {
	if c == nil || response == nil || response.Body == nil {
		return nil, errors.New("MiniMax response context is missing")
	}
	defer service.CloseResponseBodyGracefully(response)
	ctx := miniMaxRequestContext(c)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	body, err := io.ReadAll(io.LimitReader(response.Body, (128<<20)+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(body) > 128<<20 {
		return nil, errors.New("MiniMax response is too large")
	}
	return body, nil
}
func miniMaxSpeechUsage(info *relaycommon.RelayInfo, characters int64) *dto.Usage {
	prompt := info.GetEstimatePromptTokens()
	if characters > 0 {
		prompt = common.QuotaRound(float64(characters))
	}
	return &dto.Usage{PromptTokens: prompt, TotalTokens: prompt, PromptTokensDetails: dto.InputTokenDetails{TextTokens: prompt}}
}
func writeMiniMaxAudio(c *gin.Context, data []byte, format string) error {
	if err := miniMaxRequestContext(c).Err(); err != nil {
		return err
	}
	c.Header("Content-Type", getContentTypeByFormat(format))
	c.Status(http.StatusOK)
	_, err := c.Writer.Write(data)
	if err == nil {
		c.Writer.Flush()
	}
	return err
}
func handleTTSResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return nil, miniMaxExecutionError(errors.New("MiniMax speech context is missing"))
	}
	format := c.GetString("response_format")
	if format == "" {
		format = "mp3"
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		sse := c.GetString("minimax_stream_format") == "sse"
		if !sse {
			info.DisablePing = true
		}
		var failure error
		var characters int64
		var sent strings.Builder
		helper.StreamScannerHandler(c, response, info, func(raw string, result *helper.StreamResult) {
			var event MiniMaxTTSResponse
			if err := common.Unmarshal([]byte(raw), &event); err != nil {
				failure = err
				result.Stop(err)
				return
			}
			if event.BaseResp.StatusCode != 0 {
				failure = errors.New("MiniMax speech provider rejected the request")
				result.Stop(failure)
				return
			}
			if event.ExtraInfo.UsageCharacters > 0 {
				characters = event.ExtraInfo.UsageCharacters
			}
			if event.Data.Audio == "" {
				return
			}
			// stream_options excludes aggregated audio so every received buffer is a delta.
			audio, err := hex.DecodeString(event.Data.Audio)
			if err != nil {
				failure = err
				result.Stop(err)
				return
			}
			if event.Data.Status == 2 && sent.Len() > 0 && strings.HasPrefix(event.Data.Audio, sent.String()) {
				audio, err = hex.DecodeString(event.Data.Audio[sent.Len():])
				if err != nil {
					failure = err
					result.Stop(err)
					return
				}
			}
			if len(audio) == 0 {
				return
			}
			sent.WriteString(hex.EncodeToString(audio))
			if sse {
				encoded, encodeErr := common.Marshal(map[string]any{"type": "speech.audio.delta", "audio": base64.StdEncoding.EncodeToString(audio)})
				if encodeErr != nil {
					failure = encodeErr
					result.Stop(encodeErr)
					return
				}
				err = helper.StringData(c, string(encoded))
			} else {
				err = writeMiniMaxAudio(c, audio, format)
			}
			if err != nil {
				failure = err
				result.Stop(err)
			}
		})
		usage := miniMaxSpeechUsage(info, characters)
		if failure != nil {
			return usage, miniMaxExecutionError(failure)
		}
		if sent.Len() == 0 {
			return nil, miniMaxExecutionError(errors.New("MiniMax speech stream returned no audio"))
		}
		if err := miniMaxRequestContext(c).Err(); err != nil {
			return usage, miniMaxExecutionError(err)
		}
		if info.StreamStatus.HasErrors() {
			return usage, miniMaxExecutionError(errors.New("MiniMax speech stream did not complete"))
		}
		if sse {
			encoded, err := common.Marshal(map[string]any{"type": "speech.audio.done", "usage": usage})
			if err != nil {
				return usage, miniMaxExecutionError(err)
			}
			if err := helper.StringData(c, string(encoded)); err != nil {
				return usage, miniMaxExecutionError(err)
			}
		}
		return usage, nil
	}
	body, err := readMiniMaxResponse(c, response)
	if err != nil {
		return nil, miniMaxExecutionError(err)
	}
	var decoded MiniMaxTTSResponse
	if err := common.Unmarshal(body, &decoded); err != nil {
		return nil, miniMaxExecutionError(err)
	}
	if decoded.BaseResp.StatusCode != 0 {
		return nil, miniMaxExecutionError(errors.New("MiniMax speech provider rejected the request"))
	}
	if decoded.Data.Audio == "" {
		return nil, miniMaxExecutionError(errors.New("MiniMax provider returned empty audio"))
	}
	usage := miniMaxSpeechUsage(info, decoded.ExtraInfo.UsageCharacters)
	if strings.HasPrefix(decoded.Data.Audio, "https://") {
		target, err := url.Parse(decoded.Data.Audio)
		if err != nil || target.Host == "" || target.User != nil {
			return nil, miniMaxExecutionError(errors.New("MiniMax audio URL is invalid"))
		}
		if err := miniMaxRequestContext(c).Err(); err != nil {
			return usage, miniMaxExecutionError(err)
		}
		c.Redirect(http.StatusFound, target.String())
		return usage, nil
	}
	audio, err := hex.DecodeString(decoded.Data.Audio)
	if err != nil {
		return nil, miniMaxExecutionError(err)
	}
	if err := writeMiniMaxAudio(c, audio, format); err != nil {
		return usage, miniMaxExecutionError(err)
	}
	return usage, nil
}

func miniMaxImageHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if info == nil {
		return nil, miniMaxExecutionError(errors.New("MiniMax image context is missing"))
	}
	body, err := readMiniMaxResponse(c, response)
	if err != nil {
		return nil, miniMaxExecutionError(err)
	}
	var decoded MiniMaxImageResponse
	if err := common.Unmarshal(body, &decoded); err != nil {
		return nil, miniMaxExecutionError(err)
	}
	if decoded.BaseResp.StatusCode != 0 {
		return nil, miniMaxExecutionError(errors.New("MiniMax image provider rejected the request"))
	}
	result, err := responseMiniMax2OpenAIImage(&decoded, info)
	if err != nil {
		return nil, miniMaxExecutionError(err)
	}
	if len(result.Data) == 0 {
		return nil, miniMaxExecutionError(errors.New("MiniMax image provider returned no images"))
	}
	info.UpdateImageCount(int64(max(len(decoded.Data.ImageURLs), len(decoded.Data.ImageBase64))))
	encoded, err := common.Marshal(result)
	if err != nil {
		return nil, miniMaxExecutionError(err)
	}
	if err := miniMaxRequestContext(c).Err(); err != nil {
		return nil, miniMaxExecutionError(err)
	}
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)
	if _, err := c.Writer.Write(encoded); err != nil {
		return nil, miniMaxExecutionError(err)
	}
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	return &dto.Usage{}, nil
}
