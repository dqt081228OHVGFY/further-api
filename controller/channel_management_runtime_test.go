package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamModelCommitRollsBackDirectoryWhenAbilityWriteFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{Name: "transactional-directory", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: "a", Key: "unused-test-key"}
	settings := dto.ChannelOtherSettings{UpstreamModelUpdateLastDetectedModels: []string{"b"}}
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	previous := channel.OtherSettings
	callback := "test:reject-directory-ability-write"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("forced ability failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
	_, _, _, _, changed, err := applyChannelUpstreamModelUpdatesContext(context.Background(), channel, []string{"b"}, nil, nil)
	require.ErrorContains(t, err, "forced ability failure")
	require.False(t, changed)
	var saved model.Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	require.Equal(t, "a", saved.Models)
	require.Equal(t, previous, saved.OtherSettings)
	require.Equal(t, "a", channel.Models)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
}

func TestUpstreamModelCommitDoesNotOverwriteConcurrentDirectoryEdit(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{Name: "stale-directory", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: "a", Key: "unused-test-key"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamModelUpdateLastDetectedModels: []string{"b"}})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("models", "a,c").Error)
	_, _, _, _, changed, err := applyChannelUpstreamModelUpdatesContext(context.Background(), channel, []string{"b"}, nil, nil)
	require.Error(t, err)
	require.False(t, changed)
	var saved model.Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	require.Equal(t, "a,c", saved.Models)
}
