package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func RelayForAdminTokenUsage(c *gin.Context) {
	q, err := relayForQuery(c, true)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || (days != 1 && days != 7 && days != 30) {
		c.JSON(400, gin.H{"success": false, "message": "统计周期仅支持1、7或30天"})
		return
	}
	now := time.Now()
	q.StartTimestamp = model.RelayForTodayStart(now) - int64(days-1)*86400
	q.EndTimestamp = now.Unix()
	data, err := model.GetRelayForTokenUsage(q)
	relayForRespond(c, data, err)
}
