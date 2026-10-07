package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRiskAssessmentDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := model.DB
	model.DB = database
	t.Cleanup(func() { model.DB = previous; require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.RiskEvent{}, &model.RiskSnapshot{}, &model.RiskReview{}))
	return database
}

func TestRiskAssessmentJoinedPaginationKeepsLiteralSearchAndReviewStatus(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	now := time.Now().UTC().Unix()
	users := []model.User{
		{Id: 1, Username: "risk_one"}, {Id: 2, Username: "risk_hidden"},
		{Id: 3, Username: "risk_two"}, {Id: 4, Username: "risk_old"}, {Id: 5, Username: "risk%literal"},
	}
	for index := range users {
		users[index].AffCode = "risk-" + strconv.Itoa(users[index].Id)
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Delete(&users[1]).Error)
	snapshots := []model.RiskSnapshot{
		{UserID: 1, WindowDays: 7, Score: 80, Level: "critical", LastRequestAt: now},
		{UserID: 2, WindowDays: 7, Score: 99, Level: "critical", LastRequestAt: now},
		{UserID: 3, WindowDays: 7, Score: 80, Level: "critical", LastRequestAt: now},
		{UserID: 4, WindowDays: 7, Score: 100, Level: "critical", LastRequestAt: now - 8*86400},
		{UserID: 5, WindowDays: 7, Score: 30, Level: "medium", LastRequestAt: now},
	}
	require.NoError(t, db.Create(&snapshots).Error)
	require.NoError(t, SetRiskReviewStatusContext(context.Background(), 3, 9, "watching"))
	query := RiskListQuery{WindowDays: 7, Page: 1, PageSize: 1, Username: "risk_", Level: "attention"}
	page, err := ListRiskUsersContext(context.Background(), query)
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, 1, page.Items[0].UserID)
	require.Equal(t, "risk_one", page.Items[0].Username)
	require.Equal(t, "unreviewed", page.Items[0].Status)
	query.Page = 2
	page, err = ListRiskUsersContext(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 3, page.Items[0].UserID)
	require.Equal(t, "watching", page.Items[0].Status)
	query.Page, query.Status = 1, "unreviewed"
	page, err = ListRiskUsersContext(context.Background(), query)
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	query.Username, query.Level, query.Status = "risk%", "", ""
	page, err = ListRiskUsersContext(context.Background(), query)
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Equal(t, 5, page.Items[0].UserID)
}

func TestRiskDetailRefreshesOnlyRequestedWindowWithAggregateEvidence(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "detail"}).Error)
	now := time.Now().UTC()
	for i := range 4 {
		event := model.RiskEvent{UserID: 7, OccurredAt: now.Add(-time.Duration(i) * time.Minute).Unix(), PromptTokens: 10, Stream: true, StatusCode: 200}
		require.NoError(t, model.CreateRiskEventContext(context.Background(), &event))
	}
	result, err := GetRiskUserContext(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, "detail", result.Username)
	require.Equal(t, 4, result.Evidence.Requests)
	require.Equal(t, "unreviewed", result.Status)
	var count int64
	require.NoError(t, db.Model(&model.RiskSnapshot{}).Where("user_id = ? AND window_days = ?", 7, 3).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.RiskSnapshot{}).Where("user_id = ? AND window_days = ?", 7, 7).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, SetRiskReviewStatusContext(context.Background(), 7, 9, "confirmed"))
	result, err = GetRiskUserContext(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, "confirmed", result.Status)
}

func TestRiskServicesValidateAndCancelBeforeDatabaseAccess(t *testing.T) {
	previous := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = previous })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListRiskUsersContext(ctx, RiskListQuery{WindowDays: 7, Page: 1, PageSize: 20})
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetRiskUserContext(ctx, 1, 3)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, SetRiskReviewStatusContext(ctx, 1, 9, "confirmed"), context.Canceled)
	_, err = ListRiskUsersContext(context.Background(), RiskListQuery{WindowDays: 1, Page: 1, PageSize: 20})
	require.ErrorIs(t, err, ErrRiskQueryInvalid)
	require.ErrorIs(t, SetRiskReviewStatusContext(context.Background(), 1, 0, "confirmed"), ErrRiskQueryInvalid)
}
