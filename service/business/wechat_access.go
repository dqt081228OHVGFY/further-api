package business

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
)

var ErrWeChatAccessDisabled = errors.New("当前站点未开放微信登录")
var ErrWeChatCodeInvalid = errors.New("微信验证码无效或已过期")
var ErrWeChatServiceUnavailable = errors.New("微信登录服务暂不可用，请稍后重试")

type WeChatAccessAssertion struct{ Subject string }
type wechatServiceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func ExchangeWeChatCodeContext(parent context.Context, code string) (*WeChatAccessAssertion, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 1024 {
		return nil, ErrWeChatCodeInvalid
	}
	common.OptionMapRWMutex.RLock()
	enabled, address, credential := common.WeChatAuthEnabled, common.WeChatServerAddress, common.WeChatServerToken
	common.OptionMapRWMutex.RUnlock()
	if !enabled {
		return nil, ErrWeChatAccessDisabled
	}
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil {
		return nil, ErrWeChatServiceUnavailable
	}
	endpoint := base.JoinPath("api", "wechat", "user")
	values := endpoint.Query()
	values.Set("code", code)
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrWeChatServiceUnavailable
	}
	request.Header.Set("Authorization", credential)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "further-api")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrWeChatServiceUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrWeChatServiceUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<18)+1))
	if err != nil || len(body) > (1<<18) {
		return nil, ErrWeChatServiceUnavailable
	}
	var result wechatServiceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ErrWeChatServiceUnavailable
	}
	subject := strings.TrimSpace(result.Data)
	if !result.Success || subject == "" || len(subject) > 128 {
		return nil, ErrWeChatCodeInvalid
	}
	return &WeChatAccessAssertion{Subject: subject}, nil
}
