package service

import (
	"strconv"

	"github.com/dqt081228OHVGFY/further-api/model"
)

// Upstream IDs are unique within a channel account, not across accounts.
// Include the channel before sharing polling work between its goroutines.
func TaskPollingIdentity(channelID int, upstreamID string) string {
	return strconv.Itoa(channelID) + "\x00" + upstreamID
}

func taskForPolling(tasks map[string]*model.Task, channelID int, upstreamID string) *model.Task {
	if task := tasks[TaskPollingIdentity(channelID, upstreamID)]; task != nil {
		if task.ChannelId == channelID && task.GetUpstreamTaskID() == upstreamID {
			return task
		}
		return nil
	}
	// Retain the helper API's single-channel input format, but never permit it
	// to hand another channel's task to the caller or its settlement logic.
	if task := tasks[upstreamID]; task != nil && task.ChannelId == channelID && task.GetUpstreamTaskID() == upstreamID {
		return task
	}
	return nil
}
