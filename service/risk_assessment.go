package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const riskAssessmentDeadline = 10 * time.Second

var ErrRiskQueryInvalid = errors.New("风险查询参数无效。")

type riskAssessmentRow struct {
	model.RiskSnapshot `gorm:"embedded"`
	Username           string `gorm:"column:risk_username"`
	ReviewStatus       string `gorm:"column:risk_review_status"`
}

func riskAssessmentContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	if model.DB == nil {
		return nil, nil, errors.New("风险数据服务暂时不可用。")
	}
	ctx, cancel := context.WithTimeout(parent, riskAssessmentDeadline)
	return ctx, cancel, nil
}

func validRiskWindow(days int) bool { return days == 3 || days == 7 }

func validRiskReviewStatus(status string, allowEmpty bool) bool {
	switch status {
	case "unreviewed", "watching", "confirmed", "dismissed":
		return true
	case "":
		return allowEmpty
	default:
		return false
	}
}

func validateRiskListQuery(query RiskListQuery) (RiskListQuery, error) {
	query.Username = strings.TrimSpace(query.Username)
	if !validRiskWindow(query.WindowDays) || query.Page < 1 || query.Page > 10000 ||
		query.PageSize < 1 || query.PageSize > 100 || len(query.Username) > 64 ||
		!validRiskReviewStatus(query.Status, true) {
		return RiskListQuery{}, ErrRiskQueryInvalid
	}
	switch query.Level {
	case "", "attention", "low", "medium", "high", "critical":
	default:
		return RiskListQuery{}, ErrRiskQueryInvalid
	}
	return query, nil
}

// ListRiskUsersContext returns identities and dispositions from the same joined
// rows as the scores. No per-user queries or username/review lookup pass follows.
func ListRiskUsersContext(parent context.Context, input RiskListQuery) (RiskListPage, error) {
	query, err := validateRiskListQuery(input)
	if err != nil {
		return RiskListPage{}, err
	}
	ctx, cancel, err := riskAssessmentContext(parent)
	if err != nil {
		return RiskListPage{}, err
	}
	defer cancel()
	base := model.DB.WithContext(ctx).Table("risk_snapshots").
		Joins("JOIN users ON users.id = risk_snapshots.user_id").
		Joins("LEFT JOIN risk_reviews ON risk_reviews.user_id = risk_snapshots.user_id").
		Where("users.deleted_at IS NULL").
		Where("risk_snapshots.window_days = ? AND risk_snapshots.last_request_at >= ?", query.WindowDays, time.Now().UTC().AddDate(0, 0, -query.WindowDays).Unix())
	if query.Username != "" {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query.Username)
		base = base.Where("users.username LIKE ? ESCAPE '!'", "%"+literal+"%")
	}
	if query.Level == "attention" {
		base = base.Where("risk_snapshots.level IN ?", []string{"high", "critical"})
	} else if query.Level != "" {
		base = base.Where("risk_snapshots.level = ?", query.Level)
	}
	if query.Status != "" {
		base = base.Where("COALESCE(risk_reviews.status, 'unreviewed') = ?", query.Status)
	}
	page := RiskListPage{Items: make([]RiskUserAssessment, 0), Page: query.Page, PageSize: query.PageSize, Window: strconv.Itoa(query.WindowDays) + "d"}
	if err := base.Session(&gorm.Session{}).Count(&page.Total).Error; err != nil {
		return RiskListPage{}, err
	}
	if page.Total == 0 {
		return page, nil
	}
	var rows []riskAssessmentRow
	if err := base.Select("risk_snapshots.*, users.username AS risk_username, COALESCE(risk_reviews.status, 'unreviewed') AS risk_review_status").
		Order("risk_snapshots.score DESC").Order("risk_snapshots.user_id ASC").
		Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Scan(&rows).Error; err != nil {
		return RiskListPage{}, err
	}
	for _, row := range rows {
		page.Items = append(page.Items, riskAssessment(row.RiskSnapshot, row.Username, row.ReviewStatus))
	}
	return page, nil
}

func ListRiskUsers(query RiskListQuery) (RiskListPage, error) {
	return ListRiskUsersContext(context.Background(), query)
}

// Detail computes only the requested evidence window. A canceled screen no
// longer starts two unrelated snapshot refreshes with background contexts.
func GetRiskUserContext(parent context.Context, userID, windowDays int) (RiskUserAssessment, error) {
	if userID <= 0 || !validRiskWindow(windowDays) {
		return RiskUserAssessment{}, ErrRiskQueryInvalid
	}
	ctx, cancel, err := riskAssessmentContext(parent)
	if err != nil {
		return RiskUserAssessment{}, err
	}
	defer cancel()
	var identity riskAssessmentRow
	if err := model.DB.WithContext(ctx).Table("users").
		Joins("LEFT JOIN risk_reviews ON risk_reviews.user_id = users.id").
		Select("users.id AS user_id, users.username AS risk_username, COALESCE(risk_reviews.status, 'unreviewed') AS risk_review_status").
		Where("users.id = ? AND users.deleted_at IS NULL", userID).Take(&identity).Error; err != nil {
		return RiskUserAssessment{}, err
	}
	now := time.Now().UTC()
	events, err := model.ListRiskEventsContext(ctx, userID, now.AddDate(0, 0, -windowDays).Unix(), riskEventSampleLimit)
	if err != nil {
		return RiskUserAssessment{}, err
	}
	if err := ctx.Err(); err != nil {
		return RiskUserAssessment{}, err
	}
	snapshot := CalculateRiskSnapshot(userID, windowDays, events, now)
	snapshot.SampleLimited = len(events) == riskEventSampleLimit
	if err := model.UpsertRiskSnapshotContext(ctx, &snapshot); err != nil {
		return RiskUserAssessment{}, err
	}
	return riskAssessment(snapshot, identity.Username, identity.ReviewStatus), nil
}

func GetRiskUser(userID, windowDays int) (RiskUserAssessment, error) {
	return GetRiskUserContext(context.Background(), userID, windowDays)
}

func SetRiskReviewStatusContext(parent context.Context, userID, reviewerID int, input string) error {
	status := strings.TrimSpace(input)
	if userID <= 0 || reviewerID <= 0 || !validRiskReviewStatus(status, false) {
		return ErrRiskQueryInvalid
	}
	ctx, cancel, err := riskAssessmentContext(parent)
	if err != nil {
		return err
	}
	defer cancel()
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		review := model.RiskReview{UserID: userID, Status: status, ReviewedBy: reviewerID, UpdatedAt: time.Now().UTC().Unix()}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "reviewed_by", "updated_at"}),
		}).Create(&review).Error
	})
}

func SetRiskReviewStatus(userID, reviewerID int, status string) error {
	return SetRiskReviewStatusContext(context.Background(), userID, reviewerID, status)
}
