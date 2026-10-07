package model

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RiskEvent contains metadata only. It must never contain an IP address,
// request body, prompt, or a reversible representation of any of them.
type RiskEvent struct {
	ID                 int64  `gorm:"primaryKey"`
	UserID             int    `gorm:"index:idx_risk_user_time,priority:1;not null"`
	TokenID            int    `gorm:"index:idx_risk_token_time,priority:1"`
	OccurredAt         int64  `gorm:"index:idx_risk_user_time,priority:2;index:idx_risk_token_time,priority:2;index:idx_risk_time;not null"`
	Model              string `gorm:"size:64"`
	Stream             bool
	PromptTokens       int
	StatusCode         int
	QuotaExhausted     bool
	ChannelUnavailable bool
	LivenessKeywordHit bool
	IPFingerprint      string `gorm:"size:64"`
	InputSignature     string `gorm:"size:64"`
	RequestID          string `gorm:"size:64"`
}

// RiskSnapshot is the latest bounded-window assessment for one user.
// Signal values are normalized to 0..100; Score is their weighted sum.
type RiskSnapshot struct {
	UserID                         int    `gorm:"primaryKey"`
	WindowDays                     int    `gorm:"primaryKey;index:idx_risk_window_score,priority:1"`
	Score                          int    `gorm:"index:idx_risk_window_score,priority:2"`
	Level                          string `gorm:"size:16;index"`
	RegularTiming                  int
	RepeatedInput                  int
	MultiModel                     int
	Post402                        int
	NonStream                      int
	NightActivity                  int
	PostChannelUnavailable         int
	BlockedLiveness                int
	MultiIP                        int
	RequestCount                   int
	IPCount                        int
	DistinctModels                 int
	RepeatedInputs                 int
	RegularIntervals               int
	Post402Requests                int
	NonStreamRequests              int
	NightRequests                  int
	PostChannelUnavailableRequests int
	BlockedLivenessRequests        int
	SampleLimited                  bool
	LastRequestAt                  int64 `gorm:"index"`
	UpdatedAt                      int64
}

// RiskReview stores the administrator's disposition independently of score.
type RiskReview struct {
	UserID     int    `gorm:"primaryKey"`
	Status     string `gorm:"size:16;not null"`
	UpdatedAt  int64
	ReviewedBy int
}

func validRiskDigest(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func validRiskRequestID(value string) bool {
	if len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func CreateRiskEvent(event *RiskEvent) error {
	return CreateRiskEventContext(context.Background(), event)
}

func CreateRiskEventContext(ctx context.Context, event *RiskEvent) error {
	if event == nil || event.UserID <= 0 || event.OccurredAt <= 0 {
		return errors.New("invalid risk event identity or timestamp")
	}
	if !validRiskDigest(event.IPFingerprint) || !validRiskDigest(event.InputSignature) || !validRiskRequestID(event.RequestID) {
		return errors.New("invalid risk event fingerprint or request ID")
	}
	if event.Model != "" {
		mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
		_, _ = mac.Write([]byte(event.Model))
		event.Model = hex.EncodeToString(mac.Sum(nil))
	}
	if event.PromptTokens < 0 {
		event.PromptTokens = 0
	}
	return DB.WithContext(ctx).Create(event).Error
}

func ListRiskEvents(userID int, since int64, limit int) ([]RiskEvent, error) {
	return ListRiskEventsContext(context.Background(), userID, since, limit)
}

func ListRiskEventsContext(ctx context.Context, userID int, since int64, limit int) ([]RiskEvent, error) {
	events := make([]RiskEvent, 0)
	err := DB.WithContext(ctx).Where("user_id = ? AND occurred_at >= ?", userID, since).
		Order("occurred_at DESC").Order("id DESC").Limit(limit).
		Find(&events).Error
	return events, err
}

func UpsertRiskSnapshot(snapshot *RiskSnapshot) error {
	return UpsertRiskSnapshotContext(context.Background(), snapshot)
}

func UpsertRiskSnapshotContext(ctx context.Context, snapshot *RiskSnapshot) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "window_days"}},
		UpdateAll: true,
	}).Create(snapshot).Error
}

func GetRiskSnapshot(userID int, windowDays int) (*RiskSnapshot, error) {
	var snapshot RiskSnapshot
	err := DB.Where("user_id = ? AND window_days = ?", userID, windowDays).First(&snapshot).Error
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func ListRiskSnapshots(windowDays int, since int64, page int, pageSize int, username string, level string, status string) ([]RiskSnapshot, int64, error) {
	query := DB.Table("risk_snapshots").
		Joins("JOIN users ON users.id = risk_snapshots.user_id").
		Joins("LEFT JOIN risk_reviews ON risk_reviews.user_id = risk_snapshots.user_id").
		Where("risk_snapshots.window_days = ? AND risk_snapshots.last_request_at >= ?", windowDays, since).
		Where("users.deleted_at IS NULL")
	if username != "" {
		// Treat search input literally, including SQL LIKE's wildcard characters.
		pattern := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.TrimSpace(username))
		query = query.Where("users.username LIKE ? ESCAPE '!'", "%"+pattern+"%")
	}
	if level == "attention" {
		query = query.Where("risk_snapshots.level IN ?", []string{"high", "critical"})
	} else if level != "" {
		query = query.Where("risk_snapshots.level = ?", level)
	}
	if status == "unreviewed" {
		query = query.Where("(risk_reviews.status IS NULL OR risk_reviews.status = ?)", "unreviewed")
	} else if status != "" {
		query = query.Where("risk_reviews.status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	snapshots := make([]RiskSnapshot, 0)
	err := query.Select("risk_snapshots.*").Order("risk_snapshots.score DESC").
		Order("risk_snapshots.user_id ASC").Offset((page - 1) * pageSize).
		Limit(pageSize).Find(&snapshots).Error
	return snapshots, total, err
}

func RiskUserNames(userIDs []int) (map[int]string, error) {
	names := make(map[int]string, len(userIDs))
	if len(userIDs) == 0 {
		return names, nil
	}
	var users []User
	if err := DB.Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		names[user.Id] = user.Username
	}
	return names, nil
}

func RiskReviewStatuses(userIDs []int) (map[int]string, error) {
	statuses := make(map[int]string, len(userIDs))
	if len(userIDs) == 0 {
		return statuses, nil
	}
	var reviews []RiskReview
	if err := DB.Where("user_id IN ?", userIDs).Find(&reviews).Error; err != nil {
		return nil, err
	}
	for _, review := range reviews {
		statuses[review.UserID] = review.Status
	}
	return statuses, nil
}

func UpsertRiskReview(review *RiskReview) error {
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		UpdateAll: true,
	}).Create(review).Error
}

func PruneRiskEvents(before int64, limit int) (int, error) {
	return PruneRiskEventsContext(context.Background(), before, limit)
}

func PruneRiskEventsContext(ctx context.Context, before int64, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	var ids []int64
	if err := DB.WithContext(ctx).Model(&RiskEvent{}).Where("occurred_at < ?", before).
		Order("occurred_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&RiskEvent{})
	return int(result.RowsAffected), result.Error
}

// PruneRiskSnapshots removes expired aggregate evidence as its window closes.
// Review decisions are kept separately for administrators' audit history.
func PruneRiskSnapshots(now int64) error {
	return PruneRiskSnapshotsContext(context.Background(), now)
}

func PruneRiskSnapshotsContext(ctx context.Context, now int64) error {
	return DB.WithContext(ctx).Where("(window_days = ? AND last_request_at < ?) OR (window_days = ? AND last_request_at < ?)",
		3, now-3*24*3600, 7, now-7*24*3600).Delete(&RiskSnapshot{}).Error
}

func IsRiskSnapshotMissing(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
