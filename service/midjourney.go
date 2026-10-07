package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
)

func CovertMjpActionToModelName(action string) string {
	if action == constant.MjActionSwapFace {
		return "swap_face"
	}
	return "mj_" + strings.ToLower(action)
}

func PrepareMidjourneyTaskBilling(info *relaycommon.RelayInfo, task *model.Midjourney, amount int, bill bool) (bool, error) {
	if task == nil {
		return false, errors.New("Midjourney任务为空。")
	}
	task.Quota, task.TokenId, task.BillingChannelId = 0, 0, 0
	if !bill {
		return false, nil
	}
	if info == nil || !validFundingAmount(amount) {
		return false, ErrFundingAmountInvalid
	}
	if info.BillingSource == BillingSourceSubscription {
		return false, errors.New("Midjourney兼容任务仅支持钱包结算。")
	}
	task.Quota, task.BillingChannelId = amount, task.ChannelId
	if info.ChannelMeta != nil && info.ChannelId > 0 {
		task.BillingChannelId = info.ChannelId
	}
	return true, nil
}

func SettleMidjourneyTaskBilling(info *relaycommon.RelayInfo, task *model.Midjourney, prepared bool) (bool, error) {
	if !prepared {
		return false, nil
	}
	if info == nil || task == nil || task.Id <= 0 {
		return false, errors.New("Midjourney任务必须先持久化再结算。")
	}
	var receipt postConsumeQuotaResult
	var settlementErr error
	if info.Billing != nil {
		settlementErr = SettleBilling(nil, info, task.Quota)
		receipt = postConsumeQuotaResult{FundingApplied: settlementErr == nil, TokenApplied: settlementErr == nil && !info.IsPlayground}
		if session, ok := info.Billing.(*BillingSession); ok && settlementErr != nil {
			session.mu.Lock()
			receipt.FundingApplied = session.fundingSettled
			session.mu.Unlock()
		}
	} else {
		receipt, settlementErr = postConsumeQuotaWithResult(info, task.Quota, 0, true)
	}
	task.TokenId = 0
	if !receipt.FundingApplied {
		task.Quota, task.BillingChannelId = 0, 0
	}
	if receipt.TokenApplied {
		task.TokenId = info.TokenId
	}
	saveErr := task.UpdateBillingState()
	if saveErr != nil {
		saveErr = fmt.Errorf("保存Midjourney结算阶段：%w", saveErr)
	}
	return receipt.FundingApplied, errors.Join(settlementErr, saveErr)
}

func RefundMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) bool {
	if task == nil {
		return false
	}
	if task.Quota == 0 {
		return true
	}
	amount, applied, err := model.RefundMidjourneyAccountingContext(ctx, task)
	if err != nil {
		logger.LogError(ctx, "Midjourney退款事务未提交："+err.Error())
		return false
	}
	if !applied {
		return true
	}
	other := model.NewLogOther()
	other.SetPublic("task_id", task.MjId)
	other.SetPublic("reason", reason)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{UserId: task.UserId, LogType: model.LogTypeRefund,
		ChannelId: task.GetBillingChannelId(), ModelName: CovertMjpActionToModelName(task.Action), Quota: amount, TokenId: task.TokenId, Other: other})
	return true
}

var midjourneyModeActions = map[int]string{
	relayconstant.RelayModeMidjourneyImagine:  constant.MjActionImagine,
	relayconstant.RelayModeMidjourneyVideo:    constant.MjActionVideo,
	relayconstant.RelayModeMidjourneyEdits:    constant.MjActionEdits,
	relayconstant.RelayModeMidjourneyDescribe: constant.MjActionDescribe,
	relayconstant.RelayModeMidjourneyBlend:    constant.MjActionBlend,
	relayconstant.RelayModeMidjourneyShorten:  constant.MjActionShorten,
	relayconstant.RelayModeMidjourneyModal:    constant.MjActionModal,
	relayconstant.RelayModeSwapFace:           constant.MjActionSwapFace,
	relayconstant.RelayModeMidjourneyUpload:   constant.MjActionUpload,
}

func GetMjRequestModel(mode int, request *dto.MidjourneyRequest) (string, *dto.MidjourneyResponse, bool) {
	if request == nil {
		return "", MidjourneyErrorWrapper(constant.MjRequestError, "invalid_request"), false
	}
	action := midjourneyModeActions[mode]
	switch mode {
	case relayconstant.RelayModeMidjourneyAction:
		if failure := CoverPlusActionToNormalAction(request); failure != nil {
			return "", failure, false
		}
		action = request.Action
	case relayconstant.RelayModeMidjourneyChange:
		action = request.Action
	case relayconstant.RelayModeMidjourneySimpleChange:
		converted := ConvertSimpleChangeParams(request.Content)
		if converted == nil {
			return "", MidjourneyErrorWrapper(constant.MjRequestError, "invalid_request"), false
		}
		action = converted.Action
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition, relayconstant.RelayModeMidjourneyNotify:
		return "", nil, true
	}
	if action == "" {
		return "", MidjourneyErrorWrapper(constant.MjRequestError, "unknown_relay_action"), false
	}
	request.Action = action
	return CovertMjpActionToModelName(action), nil, true
}

func CoverPlusActionToNormalAction(request *dto.MidjourneyRequest) *dto.MidjourneyResponse {
	if request == nil || request.CustomId == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "custom_id_is_required")
	}
	parts := strings.Split(request.CustomId, "::")
	if len(parts) < 2 {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	position := 1
	if parts[1] == "JOB" {
		position = 2
	}
	if position >= len(parts) || parts[position] == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	action := parts[position]
	index := 1
	parseIndex := action == "variation" || strings.Contains(action, "upsample")
	if parseIndex {
		if len(parts) < 4 {
			return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
		}
		parsed, err := strconv.Atoi(parts[3])
		if err != nil || parsed < 1 || parsed > 4 {
			return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
		}
		index = parsed
	}
	mapped := ""
	switch {
	case strings.Contains(action, "upsample"):
		mapped = constant.MjActionUpscale
	case action == "variation":
		mapped = constant.MjActionVariation
	case action == "low_variation":
		mapped = constant.MjActionLowVariation
	case action == "high_variation":
		mapped = constant.MjActionHighVariation
	case strings.Contains(action, "pan"):
		mapped = constant.MjActionPan
	case strings.Contains(action, "reroll"):
		mapped = constant.MjActionReRoll
	case action == "Outpaint":
		mapped = constant.MjActionZoom
	case action == "CustomZoom":
		mapped = constant.MjActionCustomZoom
	case action == "Inpaint":
		mapped = constant.MjActionInPaint
	default:
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	request.Action, request.Index = mapped, index
	return nil
}

func ConvertSimpleChangeParams(content string) *dto.MidjourneyRequest {
	parts := strings.Fields(content)
	if len(parts) != 2 {
		return nil
	}
	action := strings.ToLower(parts[1])
	request := &dto.MidjourneyRequest{TaskId: parts[0]}
	if action == "r" {
		request.Action = constant.MjActionReRoll
		return request
	}
	if len(action) != 2 || (action[0] != 'u' && action[0] != 'v') || action[1] < '1' || action[1] > '4' {
		return nil
	}
	request.Action = constant.MjActionUpscale
	if action[0] == 'v' {
		request.Action = constant.MjActionVariation
	}
	request.Index = int(action[1] - '0')
	return request
}

func RecordMidjourneyPolicyResponse(c *gin.Context, response *dto.MidjourneyResponseWithStatusCode, requestErr error) bool {
	if response == nil {
		response = MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "empty_response", 502)
	}
	accepted := requestErr == nil && response.StatusCode == 200 && (response.Response.Code == 1 || response.Response.Code == 21 || response.Response.Code == 22)
	if accepted {
		properties, _ := response.Response.Properties.(map[string]any)
		if properties["status"] != "FAILURE" {
			return true
		}
	}
	channelID := 0
	if c != nil {
		channelID = c.GetInt("channel_id")
	}
	event := PolicyEvent{ChannelID: channelID, Status: response.StatusCode, ErrorCode: strconv.Itoa(response.Response.Code), ErrorSource: "upstream",
		Decision: PolicyDecision{Action: "failure", Reason: "upstream_failure", Source: "upstream"}}
	RequestPolicy(c).AddEvent(event)
	event.Decision, event.Health = PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}, "unchanged"
	if accepted {
		event.Decision.Reason = "task_accepted"
	}
	RequestPolicy(c).AddEvent(event)
	return false
}

func DoMidjourneyHttpRequest(c *gin.Context, timeout time.Duration, target string) (*dto.MidjourneyResponseWithStatusCode, []byte, error) {
	failure := func(status int, code string, err error) (*dto.MidjourneyResponseWithStatusCode, []byte, error) {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, code, status), nil, err
	}
	if c == nil || c.Request == nil {
		return failure(400, "invalid_request", errors.New("请求上下文无效。"))
	}
	endpoint, err := url.Parse(target)
	if err != nil || endpoint.Hostname() == "" || endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return failure(400, "create_request_failed", errors.New("渠道任务地址无效。"))
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	var payload []byte
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		var fields map[string]any
		if err := common.UnmarshalBodyReusable(c, &fields); err != nil {
			return failure(400, "read_request_body_failed", err)
		}
		if !setting.MjAccountFilterEnabled {
			delete(fields, "accountFilter")
		}
		if !setting.MjNotifyEnabled {
			delete(fields, "notifyHook")
		}
		if setting.MjModeClearEnabled {
			if prompt, ok := fields["prompt"].(string); ok {
				fields["prompt"] = strings.NewReplacer("--fast", "", "--relax", "", "--turbo", "").Replace(prompt)
			}
		}
		payload, err = common.Marshal(fields)
		if err != nil {
			return failure(400, "marshal_request_body_failed", err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, c.Request.Method, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return failure(400, "create_request_failed", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", c.Request.Header.Get("Accept"))
	key := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if key != "" {
		request.Header.Set("mj-api-secret", strings.TrimPrefix(key, "Bearer "))
	}
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return failure(502, "http_client_unavailable", err)
	}
	if ch, err := model.CacheGetChannel(c.GetInt("channel_id")); err == nil && ch.GetSetting().Proxy != "" {
		client, err = GetHttpClientWithProxy(ch.GetSetting().Proxy)
		if err != nil {
			return failure(400, "proxy_url_invalid", err)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return failure(502, "do_request_failed", err)
	}
	defer response.Body.Close()
	const responseLimit = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil || len(body) > responseLimit {
		if err == nil {
			err = errors.New("Midjourney响应超过容量限制。")
		}
		return failure(502, "read_response_body_failed", err)
	}
	envelope := &dto.MidjourneyResponseWithStatusCode{StatusCode: response.StatusCode}
	if len(body) == 0 {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "empty_response_body", response.StatusCode), body, nil
	}
	if err = common.Unmarshal(body, &envelope.Response); err != nil {
		var upload dto.MidjourneyUploadResponse
		if other := common.Unmarshal(body, &upload); other != nil {
			return failure(502, "unmarshal_response_body_failed", err)
		}
	}
	return envelope, body, nil
}
