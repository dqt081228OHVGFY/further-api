package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type systemTaskDirectoryRequest struct {
	model.SystemTaskFilter
	Offset int `form:"offset" binding:"gte=0"`
	Limit  int `form:"limit"`
}

func CreateLogCleanupSystemTask(c *gin.Context) {
	target, err := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if err != nil || target <= 0 {
		common.ApiErrorMsg(c, "请指定有效的日志清理截止时间")
		return
	}
	task, err := service.StartLogCleanupTaskContext(c.Request.Context(), target)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}

func GetCurrentSystemTask(c *gin.Context) {
	kind := strings.TrimSpace(c.Query("type"))
	if kind == "" || len(kind) > 64 {
		common.ApiErrorMsg(c, "请指定有效的系统任务类型")
		return
	}
	task, err := model.GetActiveSystemTaskContext(c.Request.Context(), kind)
	writeSystemTask(c, task, err, false)
}

func ListSystemTasks(c *gin.Context) {
	var request systemTaskDirectoryRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		writeSystemTaskFilterError(c)
		return
	}
	rows, total, err := model.ListSystemTasksContext(c.Request.Context(), request.SystemTaskFilter, request.Offset, request.Limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data := make([]model.SystemTaskResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, row.ToResponse())
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data, "total": total})
}

func DeleteSystemTaskHistory(c *gin.Context) {
	var request model.SystemTaskFilter
	if err := c.ShouldBindQuery(&request); err != nil {
		writeSystemTaskFilterError(c)
		return
	}
	deleted, err := model.DeleteSystemTaskHistoryContext(c.Request.Context(), request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted_count": deleted})
}

func GetSystemTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("task_id"))
	if id == "" || len(id) > 64 {
		common.ApiErrorMsg(c, "请指定有效的系统任务编号")
		return
	}
	task, err := model.GetSystemTaskByTaskIDContext(c.Request.Context(), id)
	writeSystemTask(c, task, err, true)
}

func writeSystemTask(c *gin.Context, task *model.SystemTask, err error, required bool) {
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		if required {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "系统任务不存在"})
		} else {
			common.ApiSuccess(c, nil)
		}
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}

func writeSystemTaskFilterError(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "系统任务筛选条件无效"})
}
