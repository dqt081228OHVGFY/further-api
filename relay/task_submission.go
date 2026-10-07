package relay

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	"github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/billing_setting"
	"github.com/dqt081228OHVGFY/further-api/types"
	"github.com/gin-gonic/gin"
)

// A submission attempt prepares and reserves its own quote before invoking
// the upstream. Persistence and presentation remain the controller's barrier.
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (_ *TaskSubmitResult, failure *dto.TaskError) {
	defer func() {
		if info != nil && info.HasChannelMeta() && failure != nil {
			secrets := service.UpstreamCredentialSecrets(info.ApiKey)
			secrets = append(secrets, service.UpstreamCredentialSecrets(info.ChannelSetting.Proxy)...)
			service.RedactUpstreamTaskError(failure, secrets...)
		}
	}()
	if c == nil || c.Request == nil || info == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("任务请求上下文无效。"), "invalid_request", http.StatusBadRequest)
	}
	if failure := taskAttemptCanceled(c); failure != nil {
		return nil, failure
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	info.InitChannelMeta(c)
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	platform, adapter := getTaskAdaptorForRequest(c, platform)
	if adapter == nil {
		code, message := TaskPlatformUnavailableError(platform)
		return nil, service.TaskErrorWrapperLocal(errors.New(message), code, http.StatusBadRequest)
	}
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}
	adapter.Init(info)
	seededModel := info.OriginModelName != ""
	if seededModel {
		if failure := mapTaskAttemptModel(c, info, info.OriginModelName); failure != nil {
			return nil, failure
		}
	}
	if failure := adapter.ValidateRequestAndSetAction(c, info); failure != nil {
		return nil, failure
	}
	if !seededModel {
		name := service.CoverTaskActionToModelName(platform, info.Action)
		if failure := mapTaskAttemptModel(c, info, name); failure != nil {
			return nil, failure
		}
	}
	if failure := taskAttemptCanceled(c); failure != nil {
		return nil, failure
	}
	if failure := priceTaskAttempt(c, info, adapter); failure != nil {
		return nil, failure
	}
	if failure := reserveTaskAttempt(c, info); failure != nil {
		return nil, failure
	}
	request, err := adapter.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}
	if owner, ok := request.(io.Closer); ok {
		defer owner.Close()
	}
	if failure := taskAttemptCanceled(c); failure != nil {
		return nil, failure
	}
	response, err := adapter.DoRequest(c, info, request)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
		defer func() {
			if failure != nil {
				service.RedactUpstreamTaskError(failure, service.UpstreamRequestSecrets(response.Request)...)
			}
		}()
	}
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusBadGateway)
	}
	if response == nil || response.Body == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("上游未返回任务响应。"), "fail_to_fetch_task", http.StatusBadGateway)
	}
	if failure := taskAttemptCanceled(c); failure != nil {
		return nil, failure
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		if readErr != nil {
			return nil, service.TaskErrorWrapper(readErr, "fail_to_fetch_task", response.StatusCode)
		}
		return nil, service.TaskErrorWrapper(errors.New(message), "fail_to_fetch_task", response.StatusCode)
	}
	parsed, failure := adapter.ParseResponse(c, response, info)
	if failure != nil {
		return nil, failure
	}
	if parsed == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("任务响应未包含提交结果。"), "plugin_submit_response_invalid", http.StatusBadGateway)
	}
	quota := settleTaskAttemptQuote(c, info, adapter, parsed)
	info.PriceData.Quota = quota
	return &TaskSubmitResult{UpstreamTaskID: parsed.UpstreamTaskID, TaskData: append([]byte(nil), parsed.TaskData...), ClientResponse: parsed.ClientResponse,
		Platform: platform, Quota: quota, Immediate: parsed.Immediate, PluginState: append([]byte(nil), parsed.PluginState...)}, nil
}

func taskAttemptCanceled(c *gin.Context) *dto.TaskError {
	if c == nil || c.Request == nil {
		return service.TaskErrorWrapperLocal(errors.New("任务请求上下文无效。"), "invalid_request", http.StatusBadRequest)
	}
	if err := c.Request.Context().Err(); err != nil {
		return service.TaskErrorWrapperLocal(err, "request_canceled", 499)
	}
	return nil
}

func mapTaskAttemptModel(c *gin.Context, info *relaycommon.RelayInfo, modelName string) *dto.TaskError {
	info.OriginModelName, info.UpstreamModelName = modelName, modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}
	return nil
}

func priceTaskAttempt(c *gin.Context, info *relaycommon.RelayInfo, adapter channel.TaskAdaptor) *dto.TaskError {
	inherited := info.PriceData.OtherRatios()
	info.TieredBillingSnapshot = nil
	pluginKey := c.GetString("task_plugin_key")
	pinnedValue, _ := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, _ := pinnedValue.(jsplugin.PinnedPlugin)
	if pinned.Plugin != nil {
		pluginKey = pinned.Plugin.Meta.Key
	}
	expression, exists := billing_setting.ResolveTaskBillingExpr(pluginKey, info.OriginModelName, info.UpstreamModelName)
	if exists || billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return quoteTaskUsageExpression(c, info, adapter, pinned, pluginKey, expression, exists)
	}
	price, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	price.ReplaceOtherRatios(inherited)
	info.PriceData = price
	var ratios map[string]float64
	if validated, ok := adapter.(channel.TaskValidatedBillingProvider); ok {
		ratios, err = validated.EstimateBillingValidated(c, info)
	} else {
		ratios = adapter.EstimateBilling(c, info)
	}
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "plugin_usage_invalid", http.StatusBadRequest)
	}
	for key, value := range ratios {
		info.PriceData.AddOtherRatio(key, value)
	}
	if !common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		quota, clamp := common.QuotaFromFloatChecked(info.PriceData.ApplyOtherRatiosToFloat(float64(price.Quota)))
		info.PriceData.Quota = quota
		noteTaskQuotaClamp(info, clamp)
	}
	info.PriceData.QuotaToPreConsume = info.PriceData.Quota
	return nil
}

func quoteTaskUsageExpression(c *gin.Context, info *relaycommon.RelayInfo, adapter channel.TaskAdaptor, pinned jsplugin.PinnedPlugin, pluginKey, expression string, exists bool) *dto.TaskError {
	provider, supported := adapter.(channel.TaskUsageFactsProvider)
	if !exists || !supported || billingexpr.UsesFixedPricing(expression) {
		return service.TaskErrorWrapper(fmt.Errorf("task model %s has no supported usage expression or meter", info.OriginModelName), "model_price_error", http.StatusBadRequest)
	}
	shared := pinned.Generation.SharedModel(info.OriginModelName) || pinned.Generation.SharedModel(info.UpstreamModelName)
	if shared && pinned.Plugin != nil {
		schema, _ := pinned.Plugin.Meta.UsageForModels(info.UpstreamModelName, info.OriginModelName)
		if !billing_setting.TaskExprCompatible(expression, schema) {
			return service.TaskErrorWrapper(fmt.Errorf("task model %s pricing is not configured for plugin %s", info.OriginModelName, pluginKey), "model_price_error", http.StatusBadRequest)
		}
	}
	var facts map[string]any
	var err error
	if validated, ok := adapter.(channel.TaskValidatedUsageFactsProvider); ok {
		facts, err = validated.ExtractUsageFactsValidated(c, info)
	} else {
		facts = provider.ExtractUsageFacts(c, info)
	}
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "plugin_usage_invalid", http.StatusBadRequest)
	}
	cost, trace, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
	if err != nil || cost < 0 || math.IsNaN(cost) {
		if err == nil {
			err = errors.New("任务计费表达式返回了无效金额。")
		}
		return service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	group := helper.HandleGroupRatio(c, info)
	quota, clamp := common.QuotaRoundChecked(cost * common.QuotaPerUnit * group.GroupRatio)
	noteTaskQuotaClamp(info, clamp)
	info.PriceData = types.PriceData{Quota: quota, QuotaToPreConsume: quota, GroupRatioInfo: group}
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: billing_setting.BillingModeTieredExpr,
		ModelName: info.OriginModelName, ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: group.GroupRatio,
		EstimatedQuotaBeforeGroup: cost * common.QuotaPerUnit, EstimatedQuotaAfterGroup: quota, EstimatedTier: trace.MatchedTier,
		QuotaPerUnit: common.QuotaPerUnit, ExprVersion: billingexpr.ExprVersion(expression), TaskUsageBilling: true, UsageFacts: facts}
	return nil
}

func reserveTaskAttempt(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info.Billing != nil {
		if service.BillingReservationCovers(info, info.PriceData.Quota) {
			return nil
		}
		if err := info.Billing.Reserve(info.PriceData.Quota); err != nil {
			return service.TaskErrorWrapperLocal(err, "task_quota_reserve_failed", http.StatusForbidden)
		}
		return nil
	}
	if !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if err := service.PreConsumeBilling(c, info.PriceData.Quota, info); err != nil {
			return service.TaskErrorFromAPIError(err)
		}
	}
	return nil
}

func settleTaskAttemptQuote(c *gin.Context, info *relaycommon.RelayInfo, adapter channel.TaskAdaptor, parsed *channel.TaskSubmitResponse) int {
	quota := info.PriceData.Quota
	if parsed.Immediate != nil && parsed.Immediate.Status == model.TaskStatusFailure {
		return 0
	}
	if snapshot := info.TieredBillingSnapshot; snapshot != nil {
		if parsed.Immediate == nil || parsed.Immediate.Status != model.TaskStatusSuccess || len(parsed.Immediate.UsageFacts) == 0 {
			return quota
		}
		settlement, facts, err := service.EvaluateTaskCompletionUsage(snapshot, parsed.Immediate.UsageFacts)
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("task immediate usage settlement failed; retaining reserved quota: %v", err))
			return quota
		}
		snapshot.UsageFacts, snapshot.EstimatedTier = facts, settlement.MatchedTier
		noteTaskQuotaClamp(info, settlement.Clamp)
		return settlement.ActualQuotaAfterGroup
	}
	if ratios := adapter.AdjustBillingOnSubmit(info, parsed.TaskData); len(ratios) > 0 {
		if adjusted, valid := recalcQuotaFromRatios(info, ratios); valid {
			info.PriceData.ReplaceOtherRatios(ratios)
			return adjusted
		}
	}
	return quota
}
