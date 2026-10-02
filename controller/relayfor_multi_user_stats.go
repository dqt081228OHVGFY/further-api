package controller

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func relayForMultiUserQuery(c *gin.Context) ([]string, int64, int64, error) {
	input := c.Query("usernames")
	parts := []string{}
	if _, supplied := c.Request.URL.Query()["usernames_json"]; supplied {
		input = c.Query("usernames_json")
		var entries []any
		if len(input) > 4096 || json.Unmarshal([]byte(input), &entries) != nil {
			return nil, 0, 0, errors.New("用户名列表必须为有效的JSON字符串数组")
		}
		for _, entry := range entries {
			name, valid := entry.(string)
			if !valid {
				return nil, 0, 0, errors.New("用户名列表只能包含字符串")
			}
			parts = append(parts, name)
		}
	} else {
		parts = strings.Split(strings.ReplaceAll(input, "，", ","), ",")
	}
	names := []string{}
	seen := map[string]bool{}
	if len(input) > 4096 {
		return nil, 0, 0, errors.New("用户名列表过长")
	}
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, 0, 0, errors.New("用户名格式无效")
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 || len(names) > 50 {
		return nil, 0, 0, errors.New("请输入1至50个用户名，使用逗号分隔")
	}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	start, startErr := time.ParseInLocation("2006-01-02", c.Query("start_time"), zone)
	end, endErr := time.ParseInLocation("2006-01-02", c.Query("end_time"), zone)
	if startErr != nil || endErr != nil {
		return nil, 0, 0, errors.New("开始与结束日期必须为YYYY-MM-DD")
	}
	endExclusive := end.AddDate(0, 0, 1)
	if start.After(end) || endExclusive.Sub(start) > 180*24*time.Hour {
		return nil, 0, 0, errors.New("结束日期不能早于开始日期，统计范围最多180天")
	}
	if start.Unix() < 0 {
		return nil, 0, 0, errors.New("日期不能早于1970年")
	}
	return names, start.Unix(), endExclusive.Unix(), nil
}

func RelayForAdminMultiUserStats(c *gin.Context) {
	names, start, end, err := relayForMultiUserQuery(c)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	data, err := model.GetRelayForMultiUserStats(names, start, end)
	relayForRespond(c, data, err)
}
