package relay

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	ClientResponse any
	Platform       constant.TaskPlatform
	Quota          int
	Immediate      *relaycommon.TaskInfo
	PluginState    []byte
}

func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c == nil || c.Request == nil || info == nil {
		return service.TaskErrorWrapperLocal(errors.New("任务来源请求无效。"), "invalid_request", 400)
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		info.Action = constant.TaskActionRemix
	}
	if info.Action == constant.TaskActionRemix {
		info.OriginTaskID = strings.TrimSpace(c.Param("video_id"))
		if info.OriginTaskID == "" {
			return service.TaskErrorWrapperLocal(errors.New("video_id is required"), "invalid_request", 400)
		}
	}
	if info.OriginTaskID == "" {
		return nil
	}
	origin, found, err := model.GetByTaskIdContext(c.Request.Context(), info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", 500)
	}
	if !found {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", 400)
	}
	var fields map[string]any
	_ = common.Unmarshal(origin.Data, &fields)
	if info.OriginModelName == "" {
		info.OriginModelName = origin.Properties.OriginModelName
		if info.OriginModelName == "" {
			info.OriginModelName = origin.Properties.UpstreamModelName
		}
		if info.OriginModelName == "" {
			info.OriginModelName, _ = fields["model"].(string)
		}
	}
	ch, err := model.GetChannelById(origin.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", 400)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("原任务所属渠道已停用。"), "task_channel_disable", 400)
	}
	info.LockedChannel = ch
	if info.ChannelId != ch.Id {
		key, _, failure := ch.GetNextEnabledKey()
		if failure != nil {
			return service.TaskErrorWrapper(failure, "channel_no_available_key", failure.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
		info.ChannelId, info.ChannelType, info.ChannelBaseUrl, info.ApiKey = ch.Id, ch.Type, ch.GetBaseURL(), key
	}
	if info.Action != constant.TaskActionRemix {
		return nil
	}
	if billing := origin.PrivateData.BillingContext; billing != nil {
		for name, ratio := range billing.OtherRatios {
			info.PriceData.AddOtherRatio(name, ratio)
		}
		return nil
	}
	seconds := 4
	if raw, ok := fields["seconds"].(string); ok {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			seconds = parsed
		}
	}
	seconds = min(seconds, relaycommon.MaxTaskDurationSeconds)
	ratio := 1.0
	if size, _ := fields["size"].(string); size == "1792x1024" || size == "1024x1792" {
		ratio = 1.666667
	}
	info.PriceData.AddOtherRatio("seconds", float64(seconds))
	info.PriceData.AddOtherRatio("size", ratio)
	return nil
}

func ApplyChannelPin(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info == nil || c == nil {
		return nil
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	if origins, ok := common.GetContextKeyType[[]*model.Task](c, constant.ContextKeyOriginTasks); ok {
		references := make([]relaycommon.OriginTaskRef, 0, len(origins))
		for _, task := range origins {
			if task != nil {
				references = append(references, relaycommon.OriginTaskRef{TaskID: task.TaskID, UpstreamTaskID: task.GetUpstreamTaskID(), Action: task.Action, Status: string(task.Status), Data: append([]byte(nil), task.Data...)})
			}
		}
		info.OriginTasks = references
	}
	pin, resolved, _ := service.GetChannelConstraints(c).ResolvedPin()
	if !resolved || pin.RetryMode != dto.PinRetrySameChannel {
		return nil
	}
	ch, err := model.CacheGetChannel(pin.ChannelId)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "origin_task_channel_disabled", 400)
	}
	if ch == nil || ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("原任务所属渠道已停用。"), "origin_task_channel_disabled", 400)
	}
	info.LockedChannel = ch
	return nil
}

func ApplyOriginTaskAffinity(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return ApplyChannelPin(c, info)
}

func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	if info == nil || len(ratios) == 0 {
		return 0, false
	}
	base := info.PriceData.RemoveOtherRatiosFromFloat(float64(info.PriceData.Quota))
	adjusted := info.PriceData
	if !adjusted.ReplaceOtherRatios(ratios) {
		return 0, false
	}
	quota, clamp := common.QuotaFromFloatChecked(adjusted.ApplyOtherRatiosToFloat(base))
	noteTaskQuotaClamp(info, clamp)
	return quota, true
}

func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if info != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

func RelayTaskFetch(c *gin.Context, mode int) *dto.TaskError {
	if mode != relayconstant.RelayModeVideoFetchByID {
		return service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", 400)
	}
	if failure := taskAttemptCanceled(c); failure != nil {
		return failure
	}
	body, failure := videoFetchByIDRespBodyBuilder(c)
	if failure != nil {
		return failure
	}
	c.Header("Content-Type", "application/json")
	if _, err := c.Writer.Write(body); err != nil {
		return service.TaskErrorWrapper(err, "copy_response_body_failed", 500)
	}
	return nil
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) ([]byte, *dto.TaskError) {
	id := c.Param("task_id")
	if id == "" {
		id = c.GetString("task_id")
	}
	task, found, err := model.GetByTaskIdContext(c.Request.Context(), c.GetInt("id"), id)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "get_task_failed", 500)
	}
	if !found || !task.ResultRetrievable() {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", 400)
	}
	openAIVideo := strings.HasPrefix(c.Request.URL.Path, "/v1/videos/")
	if body := refreshTaskForFetch(c.Request.Context(), task, openAIVideo); len(body) > 0 {
		return body, nil
	}
	if openAIVideo {
		adapter := GetTaskAdaptor(task.Platform)
		if adapter == nil {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", task.ChannelId), "invalid_channel_id", 400)
		}
		converter, supported := adapter.(channel.OpenAIVideoConverter)
		if !supported {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", task.Platform), "not_implemented", 501)
		}
		body, err := converter.ConvertToOpenAIVideo(task)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "convert_to_openai_video_failed", 500)
		}
		return body, nil
	}
	body, err := common.Marshal(dto.TaskResponse[any]{Code: "success", Data: TaskModel2Dto(task)})
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "marshal_response_failed", 500)
	}
	return body, nil
}

func refreshTaskForFetch(parent context.Context, task *model.Task, openAIVideo bool) []byte {
	if task == nil || task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure || parent.Err() != nil {
		return nil
	}
	ch, err := model.GetChannelById(task.ChannelId, true)
	if err != nil || ch.Status != common.ChannelStatusEnabled || ch.Type != constant.ChannelTypeGemini && ch.Type != constant.ChannelTypeVertexAi {
		return nil
	}
	adapter := GetTaskAdaptor(task.Platform)
	if adapter == nil {
		return nil
	}
	fetcher, supported := adapter.(channel.TaskContextFetcher)
	if !supported {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	releaseResources, resourceErr := service.AcquireTaskPollResources(ctx)
	if resourceErr != nil {
		return nil
	}
	defer releaseResources()
	base := ch.GetBaseURL()
	if base == "" {
		base = constant.GetChannelBaseURL(ch.Type)
	}
	key, _, failure := ch.GetNextEnabledKey()
	if failure != nil {
		return nil
	}
	response, err := fetcher.FetchTaskContext(ctx, base, key, task, ch.GetSetting().Proxy)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil || response == nil || response.Body == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	body, err := service.ReadTaskPollBody(response)
	if err != nil {
		return nil
	}
	result, err := adapter.ParseTaskResult(task, response, body)
	if err != nil || result == nil {
		return nil
	}
	if err = service.CommitTaskObservationContext(ctx, task, result, body, adapter); err != nil || openAIVideo {
		return nil
	}
	status := "processing"
	switch task.Status {
	case model.TaskStatusSuccess:
		status = "succeeded"
	case model.TaskStatusFailure:
		status = "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		status = "queued"
	}
	raw, _ := common.Marshal(dto.TaskResponse[any]{Code: "success", Data: map[string]any{"error": nil, "format": detectVideoFormat(body), "metadata": nil, "status": status, "task_id": task.TaskID, "url": task.GetResultURL()}})
	return raw
}

func detectVideoFormat(body []byte) string {
	var envelope struct {
		Response struct {
			Videos []struct {
				MIME string `json:"mimeType"`
			} `json:"videos"`
		} `json:"response"`
	}
	if common.Unmarshal(body, &envelope) == nil && len(envelope.Response.Videos) > 0 {
		mime := envelope.Response.Videos[0].MIME
		if mime != "" && !strings.Contains(mime, "mp4") {
			return mime
		}
	}
	return "mp4"
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	if task == nil {
		return nil
	}
	return &dto.TaskDto{ID: task.ID, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, TaskID: task.TaskID, Platform: string(task.Platform),
		UserId: task.UserId, Group: task.Group, ChannelId: task.ChannelId, Quota: task.Quota, Action: constant.NormalizeTaskAction(task.Action),
		Status: string(task.Status), FailReason: task.FailReason, ResultURL: task.GetResultURL(), SubmitTime: task.SubmitTime, StartTime: task.StartTime,
		FinishTime: task.FinishTime, Progress: task.Progress, Properties: task.Properties, Username: task.Username, Data: append([]byte(nil), task.Data...)}
}
