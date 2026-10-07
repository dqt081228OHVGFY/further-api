package middleware

import (
	"sync/atomic"
	"time"

	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

// HTTPStats 存储HTTP统计信息
type HTTPStats struct {
	activeConnections int64
}

var globalStats = &HTTPStats{}

// StatsMiddleware 统计中间件
func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now().UTC()
		c.Set("further_stats_active", true)
		// 增加活跃连接数
		atomic.AddInt64(&globalStats.activeConnections, 1)

		// 确保在请求结束时减少连接数
		defer func() {
			if value, exists := c.Get("further_request_end_callbacks"); exists {
				if callbacks, ok := value.([]func()); ok {
					for _, callback := range callbacks {
						callback()
					}
				}
			}
			atomic.AddInt64(&globalStats.activeConnections, -1)
		}()

		c.Next()
		if observation, ok := collectRiskObservation(c, startedAt); ok {
			service.RecordRiskObservation(observation)
		}
	}
}

// StatsInfo 统计信息结构
type StatsInfo struct {
	ActiveConnections int64 `json:"active_connections"`
}

// GetStats 获取统计信息
func GetStats() StatsInfo {
	return StatsInfo{
		ActiveConnections: atomic.LoadInt64(&globalStats.activeConnections),
	}
}
