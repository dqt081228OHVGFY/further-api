package model

import "gorm.io/gorm"

type FurtherMultiUserUsage struct {
	UserID           int    `json:"user_id"`
	Username         string `json:"username"`
	RequestCount     int64  `json:"request_count"`
	ErrorCount       int64  `json:"error_count"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	RequestTime      int64  `json:"request_time"`
}

type FurtherMultiUserModelUsage struct {
	FurtherMultiUserUsage
	ModelName string `json:"model_name"`
}

type FurtherMultiUserStats struct {
	Items            []FurtherMultiUserUsage      `json:"items"`
	ModelUsage       []FurtherMultiUserModelUsage `json:"model_usage"`
	UnknownUsernames []string                     `json:"unknown_usernames"`
	NoUsageUsernames []string                     `json:"no_usage_usernames"`
	StartTime        int64                        `json:"start_timestamp"`
	EndTime          int64                        `json:"end_timestamp"`
	Timezone         string                       `json:"timezone"`
}

// Exact account names resolve against the main database; usage is read from
// the log database by immutable user IDs, including calls before a rename.
func GetFurtherMultiUserStats(usernames []string, start, endExclusive int64) (FurtherMultiUserStats, error) {
	result := FurtherMultiUserStats{Items: []FurtherMultiUserUsage{}, ModelUsage: []FurtherMultiUserModelUsage{}, UnknownUsernames: []string{}, NoUsageUsernames: []string{}, StartTime: start, EndTime: endExclusive, Timezone: "Asia/Shanghai"}
	var users []User
	if err := DB.Select("id, username").Where("username IN ?", usernames).Find(&users).Error; err != nil {
		return result, err
	}
	ids := make([]int, 0, len(users))
	namesByID := make(map[int]string, len(users))
	knownNames := make(map[string]bool, len(users))
	requestedNames := make(map[string]bool, len(usernames))
	for _, username := range usernames {
		requestedNames[username] = true
	}
	for _, user := range users {
		// Keep exact-name behavior consistent even on case-insensitive SQL collations.
		if !requestedNames[user.Username] {
			continue
		}
		ids = append(ids, user.Id)
		namesByID[user.Id] = user.Username
		knownNames[user.Username] = true
	}
	for _, username := range usernames {
		if !knownNames[username] {
			result.UnknownUsernames = append(result.UnknownUsernames, username)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	query := func() *gorm.DB {
		return LOG_DB.Model(&Log{}).Where("user_id IN ? AND created_at >= ? AND created_at < ? AND type IN ?", ids, start, endExclusive, []int{LogTypeConsume, LogTypeError})
	}
	metrics := "COUNT(*) AS request_count, COALESCE(SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END),0) AS error_count, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN quota ELSE 0 END),0) AS quota, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN prompt_tokens ELSE 0 END),0) AS prompt_tokens, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN completion_tokens ELSE 0 END),0) AS completion_tokens, " +
		"COALESCE(SUM(use_time),0) * 1000 AS request_time"
	var usage []FurtherMultiUserUsage
	if err := query().Select("user_id, " + metrics).Group("user_id").Scan(&usage).Error; err != nil {
		return result, err
	}
	byName := make(map[string]FurtherMultiUserUsage, len(usage))
	for _, row := range usage {
		row.Username = namesByID[row.UserID]
		byName[row.Username] = row
	}
	for _, username := range usernames {
		if row, exists := byName[username]; exists {
			result.Items = append(result.Items, row)
		} else if knownNames[username] {
			result.NoUsageUsernames = append(result.NoUsageUsernames, username)
		}
	}
	if err := query().Select("user_id, model_name, " + metrics).Group("user_id, model_name").Order("request_count DESC, user_id ASC, model_name ASC").Scan(&result.ModelUsage).Error; err != nil {
		return result, err
	}
	for index := range result.ModelUsage {
		result.ModelUsage[index].Username = namesByID[result.ModelUsage[index].UserID]
	}
	return result, nil
}
