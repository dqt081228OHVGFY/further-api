package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	"github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/types"
	"github.com/gin-gonic/gin"
)

func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task) {
	if c == nil || info == nil || !claimUsagePublication(c, info) {
		return
	}
	other := model.NewLogOther()
	other.SetPublic("is_task", true)
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	if taskDeliveredInline(c, task) {
		other.SetPublic("task_sync", true)
	}
	price := info.PriceData
	publishTaskPriceMetadata(other, price.ModelPrice, price.ModelRatio, price.GroupRatioInfo.GroupRatio, info.TieredBillingSnapshot, price.OtherRatios())
	if price.GroupRatioInfo.HasSpecialRatio {
		other.SetPublic("user_group_ratio", price.GroupRatioInfo.GroupSpecialRatio)
	}
	if info.IsModelMapped {
		other.SetPublic("is_model_mapped", true)
		other.SetPublic("upstream_model_name", info.UpstreamModelName)
	}
	appendTaskLogInfo(task, other)
	attachQuotaSaturation(c, info, other)
	content := "操作 " + info.Action
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		content += "，按次计费"
	} else {
		parameters := make([]string, 0)
		for key, value := range price.OtherRatios() {
			if value != 1 {
				parameters = append(parameters, fmt.Sprintf("%s: %.2f", key, value))
			}
		}
		if info.TieredBillingSnapshot != nil {
			for key, value := range info.TieredBillingSnapshot.UsageFacts {
				parameters = append(parameters, fmt.Sprintf("%s: %v", key, value))
			}
		}
		sort.Strings(parameters)
		if len(parameters) > 0 {
			content += ", 计算参数：" + strings.Join(parameters, ", ")
		}
	}
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{ChannelId: info.ChannelId, ModelName: info.OriginModelName,
		TokenName: c.GetString("token_name"), Quota: price.Quota, Content: content, TokenId: info.TokenId, Group: info.UsingGroup, Other: other})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, price.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, price.Quota)
}

func publishTaskPriceMetadata(other *model.LogOther, price, ratio, group float64, snapshot *billingexpr.BillingSnapshot, ratios map[string]float64) {
	other.SetPublic("model_price", price)
	other.SetPublic("group_ratio", group)
	if ratio > 0 {
		other.SetPublic("model_ratio", ratio)
	}
	if snapshot == nil {
		setTaskImageCount(other, ratios["image_count"])
		return
	}
	other.SetPublic("billing_mode", "tiered_expr")
	other.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(snapshot.ExprString)))
	other.SetPublic("matched_tier", snapshot.EstimatedTier)
	if len(snapshot.UsageFacts) > 0 {
		other.SetPublic("usage_facts", maps.Clone(snapshot.UsageFacts))
	}
	setTaskImageCount(other, snapshot.UsageFacts["image_count"])
}

func taskBillingOther(task *model.Task) *model.LogOther {
	other := model.NewLogOther()
	if task == nil {
		return other
	}
	if billing := task.PrivateData.BillingContext; billing != nil {
		ratios := map[string]float64(nil)
		if price := taskBillingContextPriceData(billing); price != nil {
			ratios = price.OtherRatios()
		}
		publishTaskPriceMetadata(other, billing.ModelPrice, billing.ModelRatio, billing.GroupRatio, billing.TieredSnapshot, ratios)
		for key, value := range ratios {
			if !other.SetPublic(key, value) {
				common.SysError("task billing metadata key rejected: " + key)
			}
		}
	}
	if task.Properties.UpstreamModelName != "" && task.Properties.UpstreamModelName != task.Properties.OriginModelName {
		other.SetPublic("is_model_mapped", true)
		other.SetPublic("upstream_model_name", task.Properties.UpstreamModelName)
	}
	appendTaskLogInfo(task, other)
	return other
}

func appendTaskLogInfo(task *model.Task, other *model.LogOther) {
	if task == nil || other == nil {
		return
	}
	if task.TaskID != "" {
		other.SetPublic("task_id", task.TaskID)
	}
	if task.PrivateData.ResultDiscarded {
		other.SetPublic("result_discarded", true)
	}
	if task.PrivateData.Execution != nil {
		AppendTaskPluginAuditInfo(other, task.PrivateData.Execution.TaskPlugin)
	}
	if task.PrivateData.UpstreamTaskID != "" {
		other.SetRoot("upstream_task_id", task.PrivateData.UpstreamTaskID)
	}
	if task.PrivateData.NodeName != "" {
		other.SetRoot("node_name", task.PrivateData.NodeName)
	}
}

func taskDeliveredInline(c *gin.Context, task *model.Task) bool {
	if task != nil && (task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure) {
		return true
	}
	if c == nil {
		return false
	}
	value, _ := c.Get(jsplugin.ContextKeyPinnedEndpoint)
	endpoint, ok := value.(jsplugin.PinnedEndpoint)
	return ok && endpoint.Protocol == jsplugin.ProtocolOpenAIImage
}

func setTaskImageCount(other *model.LogOther, value any) {
	count, ok := value.(float64)
	if ok && count >= 0 && count <= float64(dto.MaxImageN) && !math.IsNaN(count) {
		other.SetPublic("image_count", common.QuotaRound(count))
	}
}

func taskBillingContextPriceData(billing *model.TaskBillingContext) *types.PriceData {
	if billing == nil || len(billing.OtherRatios) == 0 {
		return nil
	}
	price := &types.PriceData{}
	if !price.ReplaceOtherRatios(billing.OtherRatios) {
		return nil
	}
	return price
}

func taskModelName(task *model.Task) string {
	if task == nil {
		return ""
	}
	if billing := task.PrivateData.BillingContext; billing != nil && billing.OriginModelName != "" {
		return billing.OriginModelName
	}
	return task.Properties.OriginModelName
}

func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, total int) bool {
	if task == nil || total <= 0 {
		return false
	}
	ratio, configured, _ := ratio_setting.GetModelRatio(taskModelName(task))
	if !configured || ratio <= 0 {
		return false
	}
	group := task.Group
	if group == "" {
		if ctx == nil {
			ctx = context.Background()
		}
		var owner model.User
		if err := model.DB.WithContext(ctx).Select("group").First(&owner, task.UserId).Error; err != nil {
			return false
		}
		group = owner.Group
	}
	if group == "" {
		return false
	}
	multiplier := ratio_setting.GetGroupRatio(group)
	if special, exists := ratio_setting.GetGroupGroupRatio(group, group); exists {
		multiplier = special
	}
	other := 1.0
	if price := taskBillingContextPriceData(task.PrivateData.BillingContext); price != nil {
		other = price.OtherRatioMultiplier()
	}
	quota, clamp := common.QuotaFromFloatChecked(float64(total) * ratio * multiplier * other)
	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", total, ratio, multiplier, other)
	return settleTaskAccounting(ctx, task, quota, reason, []*common.QuotaClamp{clamp})
}

func EvaluateTaskCompletionUsage(snapshot *billingexpr.BillingSnapshot, facts map[string]any) (billingexpr.TieredResult, map[string]any, error) {
	if snapshot == nil {
		return billingexpr.TieredResult{}, nil, errors.New("任务缺少冻结计费快照。")
	}
	usage := make(map[string]any, len(snapshot.UsageFacts)+len(facts))
	for key, value := range snapshot.UsageFacts {
		usage[key] = value
	}
	for key, value := range facts {
		usage[key] = value
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: usage})
	if err == nil && (result.ActualQuotaBeforeGroup < 0 || math.IsNaN(result.ActualQuotaBeforeGroup)) {
		err = errors.New("任务完成用量计费返回无效金额。")
	}
	return result, usage, err
}
