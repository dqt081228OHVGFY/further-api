package controller

import (
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

func ListSystemInstances(c *gin.Context) {
	rows, err := model.ListSystemInstancesContext(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := common.GetTimestamp()
	data := make([]model.SystemInstanceResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, row.ToResponse(now))
	}
	common.ApiSuccess(c, data)
}

func DeleteStaleSystemInstances(c *gin.Context) {
	deleted, err := model.DeleteStaleSystemInstancesContext(c.Request.Context(), common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted_count": deleted})
}

func DeleteStaleSystemInstance(c *gin.Context) {
	node := strings.TrimSpace(c.Param("node_name"))
	if node == "" {
		common.ApiErrorMsg(c, "请指定要清理的节点")
		return
	}
	deleted, err := model.DeleteStaleSystemInstanceContext(c.Request.Context(), node, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		common.ApiErrorMsg(c, "该节点已恢复在线或已被清理")
		return
	}
	common.ApiSuccess(c, gin.H{"deleted_count": 1})
}
