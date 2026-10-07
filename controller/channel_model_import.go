package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Draft discovery uses only explicitly supplied fields. Blank edit credentials
// keep the server-side saved credential; it never appears in a response.
func buildChannelModelPreview(req fetchModelsRequest) (*model.Channel, error) {
	var ch *model.Channel
	if req.ChannelID > 0 {
		var err error
		ch, err = model.GetChannelById(req.ChannelID, true)
		if err != nil {
			return nil, errors.New("渠道不存在或无法读取，请刷新后重试。")
		}
		if req.Type > 0 {
			ch.Type = req.Type
		}
	} else {
		if req.Type <= constant.ChannelTypeUnknown || req.Type >= constant.ChannelTypeDummy {
			return nil, errors.New("请选择有效的渠道供应商。")
		}
		ch = &model.Channel{Type: req.Type}
	}
	if ch.Type == constant.ChannelTypeAdvancedCustom {
		if req.AdvancedCustom == nil && req.Settings != nil && strings.TrimSpace(*req.Settings) != "" {
			var settings dto.ChannelOtherSettings
			if common.UnmarshalJsonStr(*req.Settings, &settings) == nil && settings.AdvancedCustom != nil {
				encoded, err := common.Marshal(settings.AdvancedCustom)
				if err != nil {
					return nil, err
				}
				value := string(encoded)
				req.AdvancedCustom = &value
			}
		}
		// Preserve the advanced-route preview's established saved-key contract.
		preview, err := buildAdvancedCustomModelPreviewChannel(req)
		if err != nil {
			return nil, err
		}
		ch = preview
	}
	if req.BaseURL != nil {
		value := strings.TrimSpace(*req.BaseURL)
		ch.BaseURL = &value
	}
	if key := strings.TrimSpace(req.Key); key != "" && ch.Type != constant.ChannelTypeAdvancedCustom {
		if ch.Type != constant.ChannelTypeCodex {
			key = strings.Split(key, "\n")[0]
		}
		ch.Key = key
		ch.ChannelInfo = model.ChannelInfo{}
	}
	if req.Setting != nil {
		ch.Setting = req.Setting
	}
	if req.Settings != nil {
		ch.OtherSettings = *req.Settings
	}
	if req.HeaderOverride != nil {
		ch.HeaderOverride = req.HeaderOverride
	}
	if req.Other != nil {
		ch.Other = *req.Other
	}
	if req.Organization != nil {
		ch.OpenAIOrganization = req.Organization
	}
	if req.Proxy != nil {
		settings := ch.GetSetting()
		settings.Proxy = strings.TrimSpace(*req.Proxy)
		ch.SetSetting(settings)
	}
	if err := ch.ValidateSettings(); err != nil {
		return nil, errors.New("渠道高级配置无效，请检查 JSON、代理及自定义路由。")
	}
	return ch, nil
}

func validSelectedChannelModels(values []string) ([]string, error) {
	values = normalizeModelNames(values)
	if len(values) == 0 || len(values) > 5000 {
		return nil, errors.New("请选择 1 至 5000 个上游模型。")
	}
	for _, value := range values {
		if len(value) > 255 || strings.ContainsRune(value, ',') || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, errors.New("模型 ID 无效：不能包含逗号、控制字符或超过 255 字节。")
		}
	}
	return values, nil
}

func sameDiscoveryConfiguration(a, b *model.Channel) bool {
	return a.Type == b.Type && a.Key == b.Key && a.GetBaseURL() == b.GetBaseURL() &&
		equalStringPtr(a.Setting, b.Setting) && a.OtherSettings == b.OtherSettings &&
		equalStringPtr(a.HeaderOverride, b.HeaderOverride) && a.Other == b.Other &&
		equalStringPtr(a.OpenAIOrganization, b.OpenAIOrganization)
}

// Import is additive, revalidates the upstream selection and merges the latest
// channel row under a transaction. Routing aliases and prices are untouched.
func importChannelModelsContext(ctx context.Context, ch *model.Channel, selected []string) ([]string, []string, error) {
	selected, err := validSelectedChannelModels(selected)
	if err != nil {
		return nil, nil, err
	}
	upstream, err := fetchChannelUpstreamModelIDsContext(ctx, ch)
	if err != nil {
		return nil, nil, err
	}
	if len(subtractModelNames(selected, upstream)) > 0 {
		return nil, nil, errors.New("所选模型已不在上游目录中，请重新拉取后选择。")
	}
	var added, final []string
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.Channel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, ch.Id).Error; err != nil {
			return err
		}
		if !sameDiscoveryConfiguration(ch, &current) {
			return errors.New("渠道连接配置已更新，请重新拉取模型。")
		}
		before := normalizeModelNames(current.GetModels())
		added = subtractModelNames(selected, before)
		final = mergeModelNames(before, selected)
		if len(added) == 0 {
			return nil
		}
		current.Models = strings.Join(final, ",")
		if err := tx.Model(&model.Channel{}).Where("id = ?", current.Id).Update("models", current.Models).Error; err != nil {
			return err
		}
		return current.UpdateAbilities(tx)
	})
	return added, final, err
}

func ImportChannelUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	var body struct {
		Models []string `json:"models"`
	}
	if err != nil || id < 1 || c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "渠道或模型选择无效。"})
		return
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "渠道不存在，请刷新后重试。"})
		return
	}
	added, final, err := importChannelModelsContext(c.Request.Context(), ch, body.Models)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if len(added) > 0 {
		refreshChannelRuntimeCache()
		model.InvalidatePricingCache()
	}
	recordManageAudit(c, "channel.models_import", map[string]any{"id": id, "added_count": len(added)})
	common.ApiSuccess(c, gin.H{"id": id, "added_models": added, "models": strings.Join(final, ","), "model_ids": final})
}
