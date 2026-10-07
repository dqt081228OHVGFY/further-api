package controller

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/service/business"
	"github.com/gin-gonic/gin"
)

type PerformanceStats = business.GatewayPerformanceStats
type MemoryStats = business.GatewayMemoryStats
type DiskCacheInfo = business.GatewayDiskCacheInfo
type PerformanceConfig = business.GatewayPerformanceConfig
type LogFileInfo = business.GatewayLogFile
type LogFilesResponse = business.GatewayLogDirectory

func GetPerformanceStats(c *gin.Context) {
	stats, err := business.ReadGatewayPerformanceContext(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func ClearDiskCache(c *gin.Context) {
	if err := c.Request.Context().Err(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := common.CleanupOldDiskCacheFiles(10 * time.Minute); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "不活跃的磁盘缓存已清理"})
}
func ResetPerformanceStats(c *gin.Context) {
	if err := c.Request.Context().Err(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ResetDiskCacheStats()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "缓存统计已重置"})
}
func ForceGC(c *gin.Context) {
	if err := c.Request.Context().Err(); err != nil {
		common.ApiError(c, err)
		return
	}
	runtime.GC()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "内存整理已完成"})
}
func GetLogFiles(c *gin.Context) {
	directory, err := business.ReadGatewayLogDirectoryContext(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, directory)
}
func CleanupLogFiles(c *gin.Context) {
	value, err := strconv.Atoi(c.Query("value"))
	if err != nil {
		common.ApiErrorMsg(c, "保留数量或天数格式无效")
		return
	}
	result, err := business.ApplyGatewayLogRetentionContext(c.Request.Context(), business.GatewayLogRetentionInput{Mode: c.Query("mode"), Value: value})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": result})
		return
	}
	common.ApiSuccess(c, result)
}
