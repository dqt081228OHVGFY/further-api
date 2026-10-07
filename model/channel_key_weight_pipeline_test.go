package model

import (
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/stretchr/testify/require"
)

func TestPersistedKeyWeightChangesRealSelectionAndNeverSelectsDisabledKey(t *testing.T) {
	channel := furtherChannelFixture(t)
	channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModePolling
	require.NoError(t, channel.SaveChannelInfo())
	keys, err := GetFurtherChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	_, err = SaveFurtherChannelKey(channel.Id, keys[1].ID, FurtherChannelKeyInput{Weight: 100000, Status: 1, FurtherLimitValues: defaultFurtherLimits()}, false)
	require.NoError(t, err)
	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	heavySelections := 0
	for range 100 {
		_, index, selectionErr := stored.GetNextEnabledKey()
		require.Nil(t, selectionErr)
		if index == 1 {
			heavySelections++
		}
	}
	// Native polling would select each key 50 times. Persisted unequal weights
	// must actually influence relay selection, rather than only the dashboard.
	require.GreaterOrEqual(t, heavySelections, 95)
	_, err = SaveFurtherChannelKey(channel.Id, keys[1].ID, FurtherChannelKeyInput{Weight: 100000, Status: 2, FurtherLimitValues: defaultFurtherLimits()}, false)
	require.NoError(t, err)
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	for range 10 {
		_, index, selectionErr := stored.GetNextEnabledKey()
		require.Nil(t, selectionErr)
		require.Equal(t, 0, index)
	}
}
