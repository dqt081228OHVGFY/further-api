package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	relaytypes "github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/dqt081228OHVGFY/further-api/types"
	"github.com/gin-gonic/gin"
)

func mjFailure(message string) *dto.MidjourneyResponse {
	return service.MidjourneyErrorWrapper(constant.MjRequestError, message)
}

func RelayMidjourneyImage(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	id, parseErr := strconv.Atoi(c.Param("id"))
	if parseErr != nil || id <= 0 || strconv.Itoa(id) != c.Param("id") {
		c.JSON(404, gin.H{"error": "midjourney_task_not_found"})
		return
	}
	var task model.Midjourney
	query := model.DB.WithContext(c.Request.Context()).Where("id = ?", id)
	if !middleware.IsTaskArtifactAccess(c) {
		if c.GetInt("id") <= 0 {
			c.JSON(404, gin.H{"error": "midjourney_task_not_found"})
			return
		}
		query = query.Where("user_id = ?", c.GetInt("id"))
	}
	if err := query.First(&task).Error; err != nil || task.ImageUrl == "" {
		c.JSON(404, gin.H{"error": "midjourney_task_not_found"})
		return
	}
	var owner model.User
	if err := model.DB.WithContext(c.Request.Context()).Select("id", "status").Where("id = ?", task.UserId).First(&owner).Error; err != nil || owner.Status != common.UserStatusEnabled {
		c.JSON(404, gin.H{"error": "midjourney_task_not_found"})
		return
	}
	client := service.GetSSRFProtectedHTTPClient()
	proxy := ""
	if ch, err := model.CacheGetChannel(task.ChannelId); err == nil {
		proxy = ch.GetSetting().Proxy
	}
	if proxy != "" {
		var err error
		client, err = service.GetSSRFProtectedHTTPClientWithProxy(proxy)
		if err != nil {
			c.JSON(400, gin.H{"error": "proxy_url_invalid"})
			return
		}
		config := system_setting.GetFetchSetting()
		if err := common.ValidateURLWithFetchSetting(task.ImageUrl, config.EnableSSRFProtection, config.AllowPrivateIp, config.DomainFilterMode, config.IpFilterMode, config.DomainList, config.IpList, config.AllowedPorts, config.ApplyIPFilterForDomain); err != nil {
			c.JSON(403, gin.H{"error": "图片地址不允许访问。"})
			return
		}
	} else if err := service.ValidateSSRFProtectedFetchURL(task.ImageUrl); err != nil {
		c.JSON(403, gin.H{"error": "图片地址不允许访问。"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, c.Request.Method, task.ImageUrl, nil)
	if err != nil {
		c.JSON(400, gin.H{"error": "图片地址无效。"})
		return
	}
	response, err := client.Do(request)
	if err != nil {
		if c.Request.Context().Err() == nil {
			c.JSON(502, gin.H{"error": "图片暂时无法加载。"})
		}
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		c.JSON(response.StatusCode, gin.H{"error": "图片服务暂时不可用。"})
		return
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	mediaType, _, mimeErr := mime.ParseMediaType(contentType)
	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif", "image/bmp", "image/tiff", "image/heic", "image/heif":
	default:
		mimeErr = errors.New("unsupported image content type")
	}
	maxBytes := int64(constant.MaxFileDownloadMB) << 20
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	if mimeErr != nil || response.ContentLength > maxBytes {
		c.JSON(502, gin.H{"error": "图片内容不可用。"})
		return
	}
	c.Header("Content-Type", contentType)
	if response.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	if c.Request.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(c.Writer, io.LimitReader(response.Body, maxBytes)); err != nil && c.Request.Context().Err() == nil {
		logger.LogWarn(c, "Midjourney图片传输中断："+err.Error())
	}
}

func RelayMidjourneyNotify(c *gin.Context) *dto.MidjourneyResponse {
	var incoming dto.MidjourneyDto
	if err := common.UnmarshalBodyReusable(c, &incoming); err != nil || incoming.MjId == "" {
		return mjFailure("bind_request_body_failed")
	}
	var task model.Midjourney
	query := model.DB.WithContext(c.Request.Context()).Where("mj_id = ?", incoming.MjId)
	if userID := c.GetInt("id"); userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&task).Error; err != nil {
		return mjFailure("midjourney_task_not_found")
	}
	if task.Status == "SUCCESS" || task.Status == "FAILURE" {
		return nil
	}
	from := task.Status
	for _, field := range []struct {
		target *string
		value  string
	}{
		{&task.Progress, incoming.Progress}, {&task.PromptEn, incoming.PromptEn}, {&task.State, incoming.State},
		{&task.ImageUrl, incoming.ImageUrl}, {&task.VideoUrl, incoming.VideoUrl}, {&task.Status, incoming.Status}, {&task.FailReason, incoming.FailReason},
	} {
		if field.value != "" {
			*field.target = field.value
		}
	}
	if incoming.SubmitTime > 0 {
		task.SubmitTime = incoming.SubmitTime
	}
	if incoming.StartTime > 0 {
		task.StartTime = incoming.StartTime
	}
	if incoming.FinishTime > 0 {
		task.FinishTime = incoming.FinishTime
	}
	if incoming.VideoUrls != nil {
		raw, err := common.Marshal(incoming.VideoUrls)
		if err != nil {
			return mjFailure("invalid_video_urls")
		}
		task.VideoUrls = string(raw)
	}
	terminal := task.Status == "SUCCESS" || task.Status == "FAILURE"
	if terminal {
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = time.Now().UnixMilli()
		}
	}
	updated := model.DB.WithContext(c.Request.Context()).Model(&task).Where("status = ?", from).Select("*").Updates(&task)
	if updated.Error != nil {
		return mjFailure("update_midjourney_task_failed")
	}
	if updated.RowsAffected > 0 && task.Status == "FAILURE" {
		service.RefundMidjourneyQuota(c.Request.Context(), &task, task.FailReason)
	}
	return nil
}

func coverMidjourneyTaskDto(c *gin.Context, task *model.Midjourney) dto.MidjourneyDto {
	result := dto.MidjourneyDto{MjId: task.MjId, Progress: task.Progress, PromptEn: task.PromptEn, State: task.State,
		SubmitTime: task.SubmitTime, StartTime: task.StartTime, FinishTime: task.FinishTime, ImageUrl: task.ImageUrl, VideoUrl: task.VideoUrl,
		Status: task.Status, FailReason: task.FailReason, Action: task.Action, Description: task.Description, Prompt: task.Prompt}
	if task.ImageUrl != "" && setting.MjForwardUrlEnabled {
		result.ImageUrl, _ = service.BuildMidjourneyImageContentURL(task.Id)
		if task.Status != "SUCCESS" {
			result.ImageUrl += "&rand=" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
	}
	if task.Buttons != "" {
		var buttons []dto.ActionButton
		if common.Unmarshal([]byte(task.Buttons), &buttons) == nil {
			result.Buttons = buttons
		}
	}
	if task.VideoUrls != "" {
		_ = common.Unmarshal([]byte(task.VideoUrls), &result.VideoUrls)
	}
	if task.Properties != "" {
		var properties dto.Properties
		if common.Unmarshal([]byte(task.Properties), &properties) == nil {
			result.Properties = &properties
		}
	}
	return result
}

func mjOwnedTask(c *gin.Context, id string) (*model.Midjourney, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("task_id_is_required")
	}
	var task model.Midjourney
	if err := model.DB.WithContext(c.Request.Context()).Where("user_id = ? AND mj_id = ?", c.GetInt("id"), id).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func bindMjOrigin(c *gin.Context, info *relaycommon.RelayInfo, task *model.Midjourney) *dto.MidjourneyResponse {
	ch, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return mjFailure("get_channel_info_failed")
	}
	if ch.Status != common.ChannelStatusEnabled {
		return mjFailure("该任务所属渠道已被禁用")
	}
	key, _, failure := ch.GetNextEnabledKey()
	if failure != nil {
		return mjFailure("原任务渠道没有可用密钥。")
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
	common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	if info != nil {
		info.InitChannelMeta(c)
	}
	return nil
}

func RelayMidjourneyTaskImageSeed(c *gin.Context) *dto.MidjourneyResponse {
	task, err := mjOwnedTask(c, c.Param("id"))
	if err != nil {
		return mjFailure("task_no_found")
	}
	if failure := bindMjOrigin(c, nil, task); failure != nil {
		return failure
	}
	response, raw, err := service.DoMidjourneyHttpRequest(c, 30*time.Second, strings.TrimRight(c.GetString("base_url"), "/")+getMjRequestPath(c.Request.URL.String()))
	if err != nil {
		return &response.Response
	}
	return writeMidjourneyBytes(c, response.StatusCode, raw)
}

func RelayMidjourneyTask(c *gin.Context, mode int) *dto.MidjourneyResponse {
	var result any
	switch mode {
	case relayconstant.RelayModeMidjourneyTaskFetch:
		task, err := mjOwnedTask(c, c.Param("id"))
		if err != nil {
			return mjFailure("task_no_found")
		}
		result = coverMidjourneyTaskDto(c, task)
	case relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		var condition struct {
			IDs []string `json:"ids"`
		}
		if err := common.UnmarshalBodyReusable(c, &condition); err != nil || len(condition.IDs) > 200 {
			return mjFailure("invalid_task_ids")
		}
		items := make([]dto.MidjourneyDto, 0)
		if len(condition.IDs) > 0 {
			var tasks []*model.Midjourney
			if err := model.DB.WithContext(c.Request.Context()).Where("user_id = ? AND mj_id IN ?", c.GetInt("id"), condition.IDs).Find(&tasks).Error; err != nil {
				return mjFailure("get_task_failed")
			}
			for _, task := range tasks {
				items = append(items, coverMidjourneyTaskDto(c, task))
			}
		}
		result = items
	default:
		return mjFailure("invalid_relay_mode")
	}
	body, err := common.Marshal(result)
	if err != nil {
		return mjFailure("marshal_response_body_failed")
	}
	return writeMidjourneyBytes(c, 200, body)
}

func RelaySwapFace(c *gin.Context, info *relaycommon.RelayInfo) *dto.MidjourneyResponse {
	var request dto.SwapFaceRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return mjFailure("bind_request_body_failed")
	}
	if request.SourceBase64 == "" || request.TargetBase64 == "" {
		return mjFailure("sour_base64_and_target_base64_is_required")
	}
	info.InitChannelMeta(c)
	return submitMidjourneyAttempt(c, info, &dto.MidjourneyRequest{Action: constant.MjActionSwapFace, Prompt: "InsightFace"}, true)
}

func RelayMidjourneySubmit(c *gin.Context, info *relaycommon.RelayInfo) *dto.MidjourneyResponse {
	if info == nil {
		return mjFailure("invalid_request")
	}
	var request dto.MidjourneyRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return mjFailure("bind_request_body_failed")
	}
	info.InitChannelMeta(c)
	name, failure, valid := service.GetMjRequestModel(info.RelayMode, &request)
	if !valid {
		return failure
	}
	if request.Action == "" {
		request.Action = strings.ToUpper(strings.TrimPrefix(name, "mj_"))
	}
	if info.RelayMode == relayconstant.RelayModeMidjourneyImagine && strings.TrimSpace(request.Prompt) == "" {
		return mjFailure("prompt_is_required")
	}
	dependency := request.TaskId
	if info.RelayMode == relayconstant.RelayModeMidjourneySimpleChange {
		converted := service.ConvertSimpleChangeParams(request.Content)
		if converted == nil {
			return mjFailure("content_parse_failed")
		}
		request.Action, request.TaskId, request.Index = converted.Action, converted.TaskId, converted.Index
		dependency = request.TaskId
	}
	dependent := info.RelayMode == relayconstant.RelayModeMidjourneyChange || info.RelayMode == relayconstant.RelayModeMidjourneyAction || info.RelayMode == relayconstant.RelayModeMidjourneySimpleChange || info.RelayMode == relayconstant.RelayModeMidjourneyModal
	if dependent || dependency != "" && info.RelayMode == relayconstant.RelayModeMidjourneyVideo {
		if dependency == "" {
			return mjFailure("task_id_is_required")
		}
		if info.RelayMode == relayconstant.RelayModeMidjourneyChange && (request.Action == "" || request.Index <= 0) {
			return mjFailure("action_and_index_are_required")
		}
		origin, err := mjOwnedTask(c, dependency)
		if err != nil {
			return mjFailure("task_not_found")
		}
		if setting.MjActionCheckSuccessEnabled && origin.Status != "SUCCESS" && info.RelayMode != relayconstant.RelayModeMidjourneyModal {
			return mjFailure("task_status_not_success")
		}
		if failure := bindMjOrigin(c, info, origin); failure != nil {
			return failure
		}
		request.Prompt = origin.Prompt
	}
	bill := request.Action != constant.MjActionInPaint && request.Action != constant.MjActionCustomZoom
	return submitMidjourneyAttempt(c, info, &request, bill)
}

func submitMidjourneyAttempt(c *gin.Context, info *relaycommon.RelayInfo, request *dto.MidjourneyRequest, bill bool) *dto.MidjourneyResponse {
	info.OriginModelName = service.CovertMjpActionToModelName(request.Action)
	price, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return mjFailure(err.Error())
	}
	info.PriceData = price
	if bill && !price.FreeModel {
		preferred := info.UserSetting.BillingPreference
		info.UserSetting.BillingPreference = "wallet_only"
		info.ForcePreConsume = true
		failure := service.PreConsumeBilling(c, price.Quota, info)
		info.UserSetting.BillingPreference = preferred
		if failure != nil {
			return mjFailure(failure.MaskSensitiveError())
		}
		defer func() {
			if info.Billing != nil && info.Billing.NeedsRefund() {
				info.Billing.Refund(c)
			}
		}()
	}
	service.RequestPolicy(c).BeginAttempt(&model.Channel{Id: info.ChannelId}, info.UsingGroup)
	target := strings.TrimRight(info.ChannelBaseUrl, "/") + getMjRequestPath(c.Request.URL.String())
	response, raw, err := service.DoMidjourneyHttpRequest(c, 60*time.Second, target)
	accepted := service.RecordMidjourneyPolicyResponse(c, response, err)
	if err != nil {
		return &response.Response
	}
	parsed := response.Response
	if parsed.Code == 21 && parsed.Result != "" {
		if _, err := mjOwnedTask(c, parsed.Result); err == nil {
			service.MarkRequestPolicySuccess(c, nil)
			return writeMidjourneyBytes(c, response.StatusCode, normalizeMidjourneyResponseCode(raw, request.Action))
		}
	}
	now := time.Now().UnixMilli()
	task := &model.Midjourney{UserId: info.UserId, Code: parsed.Code, Action: request.Action, MjId: parsed.Result, Prompt: request.Prompt,
		Description: parsed.Description, State: request.State, SubmitTime: now, Progress: "0%", ChannelId: info.ChannelId}
	if request.Action == constant.MjActionSwapFace {
		task.StartTime = now
	}
	if !accepted {
		task.FailReason = parsed.Description
		bill = false
	}
	if parsed.Code == 21 {
		if properties, ok := parsed.Properties.(map[string]any); ok {
			task.ImageUrl, _ = properties["imageUrl"].(string)
			task.Status, _ = properties["status"].(string)
		}
		if task.Status == "SUCCESS" {
			task.Progress, task.StartTime, task.FinishTime = "100%", now, now
		}
	}
	if parsed.Code == 1 && request.Action == constant.MjActionUpload {
		task.Status, task.Progress, task.FinishTime = "SUCCESS", "100%", now
	}
	disableEmptyMidjourneyChannel(c, info, parsed.Code)
	prepared, err := service.PrepareMidjourneyTaskBilling(info, task, price.Quota, bill && response.StatusCode == 200)
	if err != nil {
		return mjFailure(err.Error())
	}
	if err := model.DB.WithContext(c.Request.Context()).Create(task).Error; err != nil {
		return mjFailure("insert_midjourney_task_failed")
	}
	applied, billingErr := service.SettleMidjourneyTaskBilling(info, task, prepared)
	if applied {
		recordMidjourneyConsumption(c, info, task, price)
	}
	if billingErr != nil {
		logger.LogError(c, "Midjourney结算需要核查："+billingErr.Error())
	}
	if accepted {
		service.MarkRequestPolicySuccess(c, nil)
	}
	return writeMidjourneyBytes(c, response.StatusCode, normalizeMidjourneyResponseCode(raw, request.Action))
}

func recordMidjourneyConsumption(c *gin.Context, info *relaycommon.RelayInfo, task *model.Midjourney, price types.PriceData) {
	other := service.GenerateMjOtherInfo(info, price)
	service.AppendRelayLogAdminInfo(c, info, other)
	other.SetPublic("task_id", task.MjId)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{ChannelId: task.GetBillingChannelId(), ModelName: info.OriginModelName,
		TokenName: c.GetString("token_name"), Quota: task.Quota, Content: fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s，ID %s", price.ModelPrice, price.GroupRatioInfo.GroupRatio, task.Action, task.MjId), TokenId: task.TokenId, Group: info.UsingGroup, Other: other})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, task.Quota)
	model.UpdateChannelUsedQuota(task.GetBillingChannelId(), task.Quota)
}

func disableEmptyMidjourneyChannel(c *gin.Context, info *relaycommon.RelayInfo, code int) {
	if code != 3 || !common.AutomaticDisableChannelEnabled {
		return
	}
	ch, err := model.GetChannelById(info.ChannelId, true)
	if err == nil && ch.GetAutoBan() {
		service.DisableChannel(*relaytypes.NewChannelError(ch.Id, ch.Type, ch.Name, ch.ChannelInfo.IsMultiKey, "", true), "Midjourney没有可用实例。")
	}
}

func normalizeMidjourneyResponseCode(raw []byte, action string) []byte {
	var fields map[string]any
	if common.Unmarshal(raw, &fields) != nil {
		return raw
	}
	code, _ := fields["code"].(float64)
	if code == 22 || code == 21 && action != constant.MjActionInPaint && action != constant.MjActionCustomZoom {
		fields["code"] = 1
		if changed, err := common.Marshal(fields); err == nil {
			return changed
		}
	}
	return raw
}

func writeMidjourneyBytes(c *gin.Context, status int, body []byte) *dto.MidjourneyResponse {
	c.Header("Content-Type", "application/json")
	c.Status(status)
	if _, err := c.Writer.Write(body); err != nil {
		return mjFailure("copy_response_body_failed")
	}
	return nil
}

func getMjRequestPath(path string) string {
	if position := strings.Index(path, "/mj/"); position >= 0 {
		return path[position:]
	}
	return path
}
