package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChannelDraftInput struct {
	Type               int     `json:"type"`
	Key                string  `json:"key"`
	Name               string  `json:"name"`
	Status             int     `json:"status"`
	OpenAIOrganization *string `json:"openai_organization"`
	TestModel          *string `json:"test_model"`
	Weight             *uint   `json:"weight"`
	BaseURL            *string `json:"base_url"`
	Other              string  `json:"other"`
	Models             string  `json:"models"`
	Group              string  `json:"group"`
	ModelMapping       *string `json:"model_mapping"`
	StatusCodeMapping  *string `json:"status_code_mapping"`
	Priority           *int64  `json:"priority"`
	AutoBan            *int    `json:"auto_ban"`
	Tag                *string `json:"tag"`
	Setting            *string `json:"setting"`
	ParamOverride      *string `json:"param_override"`
	HeaderOverride     *string `json:"header_override"`
	Remark             *string `json:"remark"`
	OtherSettings      string  `json:"settings"`
}

type AddChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *ChannelDraftInput    `json:"channel"`
}

func (input ChannelDraftInput) record() model.Channel {
	status := input.Status
	if status == 0 {
		status = common.ChannelStatusEnabled
	}
	return model.Channel{Type: input.Type, Key: input.Key, Name: strings.TrimSpace(input.Name), Status: status, OpenAIOrganization: input.OpenAIOrganization, TestModel: input.TestModel, Weight: input.Weight, BaseURL: input.BaseURL, Other: input.Other, Models: input.Models, Group: input.Group, ModelMapping: input.ModelMapping, StatusCodeMapping: input.StatusCodeMapping, Priority: input.Priority, AutoBan: input.AutoBan, Tag: input.Tag, Setting: input.Setting, ParamOverride: input.ParamOverride, HeaderOverride: input.HeaderOverride, Remark: input.Remark, OtherSettings: input.OtherSettings, CreatedTime: common.GetTimestamp()}
}

func createChannelKeys(channel *model.Channel, mode string) ([]string, error) {
	if mode == "single" {
		return []string{channel.Key}, nil
	}
	if mode != "batch" && mode != "multi_to_single" {
		return nil, fmt.Errorf("不支持的渠道创建方式")
	}
	if channel.Type == constant.ChannelTypeVertexAi && channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
		return getVertexArrayKeys(channel.Key)
	}
	keys := make([]string, 0)
	seen := make(map[string]bool)
	for _, key := range strings.Split(channel.Key, "\n") {
		key = strings.TrimSpace(key)
		if key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("请提供有效的渠道凭证")
	}
	return keys, nil
}

func AddChannel(c *gin.Context) {
	var input AddChannelRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.Channel == nil {
		common.ApiErrorMsg(c, "渠道参数无效")
		return
	}
	channel := input.Channel.record()
	if (channel.Type == constant.ChannelTypeTaskPlugin || len(channel.GetSetting().TaskPluginBindings()) > 0) && !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.TaskPluginBind) {
		common.ApiErrorMsg(c, "task plugin channels require the task_plugin.bind permission")
		return
	}
	pluginDefault := channel.Type == constant.ChannelTypeTaskPlugin && (channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "")
	if err := validateChannel(&channel, true); err != nil {
		common.ApiError(c, err)
		return
	}
	keys, err := createChannelKeys(&channel, input.Mode)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if input.Mode == "multi_to_single" {
		channel.Key = strings.Join(keys, "\n")
		channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys), MultiKeyMode: input.MultiKeyMode}
		keys = []string{channel.Key}
	}
	channels := make([]model.Channel, len(keys))
	for index, key := range keys {
		channels[index] = channel
		channels[index].Key = key
		if input.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			prefix := key[:min(len(key), 8)]
			channels[index].Name = fmt.Sprintf("%s %s", channel.Name, prefix)
		}
	}
	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	params := map[string]any{"name": channel.Name, "type": channel.Type, "count": len(channels)}
	if pluginDefault {
		params["base_url_source"] = "plugin_default"
	}
	recordManageAudit(c, "channel.create", params)
	common.ApiSuccess(c, nil)
}

func DeleteChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "渠道编号无效")
		return
	}
	channel, err := model.GetChannelById(id, false)
	missing := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !missing {
		common.ApiError(c, err)
		return
	}
	if missing {
		channel = &model.Channel{Id: id}
	}
	if err := channel.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	if missing {
		service.ResetProxyClientCache()
	} else {
		service.InvalidateProxyClient(channel.GetSetting().Proxy)
	}
	closeActiveChannelWebSockets([]int{id})
	recordManageAudit(c, "channel.delete", map[string]any{"id": id, "name": channel.Name})
	common.ApiSuccess(c, nil)
}
