package controller

import (
	"strconv"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

func FurtherAdminTokenUsage(c *gin.Context) {
	q, err := furtherQuery(c, true)
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
	q.StartTimestamp = model.FurtherTodayStart(now) - int64(days-1)*86400
	q.EndTimestamp = now.Unix()
	data, err := model.GetFurtherTokenUsage(q)
	furtherRespond(c, data, err)
}
