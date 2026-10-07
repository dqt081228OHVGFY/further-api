package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
)

type WorkspaceSiteSettings struct {
	SystemName              string `json:"system_name"`
	Logo                    string `json:"logo"`
	ServerAddress           string `json:"server_address"`
	LinuxDOEnabled          bool   `json:"linuxdo_enabled"`
	LinuxDOClientID         string `json:"linuxdo_client_id"`
	LinuxDOSecretConfigured bool   `json:"linuxdo_client_secret_configured"`
	LinuxDOMinimumTrust     int    `json:"linuxdo_minimum_trust_level"`
	LinuxDORedirectURL      string `json:"linuxdo_redirect_url"`
}

// Pointer fields distinguish an omitted change from an intentional clear.
// Credentials are write-only and never enter the response DTO.
type WorkspaceSiteChange struct {
	SystemName          *string `json:"system_name,omitempty"`
	Logo                *string `json:"logo,omitempty"`
	ServerAddress       *string `json:"server_address,omitempty"`
	LinuxDOEnabled      *bool   `json:"linuxdo_enabled,omitempty"`
	LinuxDOClientID     *string `json:"linuxdo_client_id,omitempty"`
	LinuxDOClientSecret *string `json:"linuxdo_client_secret,omitempty"`
	LinuxDOMinimumTrust *int    `json:"linuxdo_minimum_trust_level,omitempty"`
}

type WorkspaceSiteInputError struct{ Message string }

func (err *WorkspaceSiteInputError) Error() string { return err.Message }

var workspaceSiteWrite sync.Mutex

func ReadWorkspaceSiteSettings(ctx context.Context, callback string) (WorkspaceSiteSettings, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceSiteSettings{}, err
	}
	common.OptionMapRWMutex.RLock()
	trust, _ := strconv.Atoi(common.OptionMap["LinuxDOMinimumTrustLevel"])
	result := WorkspaceSiteSettings{
		SystemName: common.ProductName, Logo: common.ProductLogo,
		ServerAddress:           common.OptionMap["ServerAddress"],
		LinuxDOEnabled:          common.OptionMap["LinuxDOOAuthEnabled"] == "true",
		LinuxDOClientID:         common.OptionMap["LinuxDOClientId"],
		LinuxDOSecretConfigured: common.LinuxDOClientSecret != "",
		LinuxDOMinimumTrust:     trust, LinuxDORedirectURL: callback,
	}
	common.OptionMapRWMutex.RUnlock()
	return result, nil
}

func SaveWorkspaceSiteSettings(ctx context.Context, change WorkspaceSiteChange) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	values := make(map[string]string)
	for key, value := range map[string]*string{"SystemName": change.SystemName, "Logo": change.Logo} {
		if value == nil {
			continue
		}
		canonical, _ := common.CanonicalBrandingValue(key)
		if strings.TrimSpace(*value) != canonical {
			return &WorkspaceSiteInputError{Message: "网关品牌固定为 further-api，请重新读取配置。"}
		}
		values[key] = canonical
	}
	if change.ServerAddress != nil {
		parsed, err := url.Parse(strings.TrimSpace(*change.ServerAddress))
		if err != nil || parsed.Host == "" || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return &WorkspaceSiteInputError{Message: "站点地址需要填写完整的 HTTP 或 HTTPS 来源地址，不含账号、路径或查询参数。"}
		}
		values["ServerAddress"] = parsed.Scheme + "://" + parsed.Host
	}
	if change.LinuxDOClientID != nil {
		value := strings.TrimSpace(*change.LinuxDOClientID)
		if len(value) > 256 || strings.ContainsAny(value, "\r\n") {
			return &WorkspaceSiteInputError{Message: "Linux DO Client ID 格式有误。"}
		}
		values["LinuxDOClientId"] = value
	}
	if change.LinuxDOClientSecret != nil {
		value := strings.TrimSpace(*change.LinuxDOClientSecret)
		if len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
			return &WorkspaceSiteInputError{Message: "Linux DO Client Secret 格式有误。"}
		}
		if value != "" {
			values["LinuxDOClientSecret"] = value
		}
	}
	if change.LinuxDOMinimumTrust != nil {
		if *change.LinuxDOMinimumTrust < 0 || *change.LinuxDOMinimumTrust > 4 {
			return &WorkspaceSiteInputError{Message: "Linux DO 最低信任等级必须在 0 到 4 之间。"}
		}
		values["LinuxDOMinimumTrustLevel"] = strconv.Itoa(*change.LinuxDOMinimumTrust)
	}
	if change.LinuxDOEnabled != nil {
		values["LinuxDOOAuthEnabled"] = strconv.FormatBool(*change.LinuxDOEnabled)
	}
	if len(values) == 0 {
		return &WorkspaceSiteInputError{Message: "没有提交需要保存的设置。"}
	}
	workspaceSiteWrite.Lock()
	defer workspaceSiteWrite.Unlock()
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["LinuxDOOAuthEnabled"] == "true"
	clientID, secret := common.OptionMap["LinuxDOClientId"], common.LinuxDOClientSecret
	common.OptionMapRWMutex.RUnlock()
	if value, changed := values["LinuxDOOAuthEnabled"]; changed {
		enabled = value == "true"
	}
	if value, changed := values["LinuxDOClientId"]; changed {
		clientID = value
	}
	if value, changed := values["LinuxDOClientSecret"]; changed {
		secret = value
	}
	if enabled && (clientID == "" || secret == "") {
		return &WorkspaceSiteInputError{Message: "启用 Linux DO 登录需要先配置 Client ID 和 Client Secret。"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if model.DB == nil {
		return errors.New("site configuration storage is unavailable")
	}
	if err := model.UpdateOptionsBulkContext(ctx, values); err != nil {
		return fmt.Errorf("save site configuration: %w", err)
	}
	return nil
}
