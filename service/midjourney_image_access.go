package service

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

const MidjourneyImageArtifactKey = "image"

// MidjourneyImageAccessTaskID isolates legacy image capabilities from generic
// task artifacts. Database row IDs bind duplicate upstream job IDs to one owner.
func MidjourneyImageAccessTaskID(id string) string {
	parsed, err := strconv.Atoi(id)
	if err != nil || parsed <= 0 || strconv.Itoa(parsed) != id {
		return ""
	}
	return "midjourney:" + id
}

func BuildMidjourneyImageContentURL(id int) (string, error) {
	taskID := MidjourneyImageAccessTaskID(strconv.Itoa(id))
	if taskID == "" {
		return "", ErrTaskArtifactAccessInvalid
	}
	baseAddress := strings.TrimSpace(system_setting.TaskPublicAddress)
	if baseAddress == "" {
		baseAddress = strings.TrimSpace(system_setting.ServerAddress)
	}
	if err := ValidateTaskArtifactBaseURL(baseAddress); err != nil {
		return "", err
	}
	base, err := url.Parse(baseAddress)
	if err != nil {
		return "", err
	}
	access, err := IssueTaskArtifactAccess(taskID, MidjourneyImageArtifactKey)
	if err != nil {
		return "", err
	}
	escapedPrefix := strings.TrimRight(base.EscapedPath(), "/")
	suffix := "/mj/image/" + strconv.Itoa(id)
	base.Path = strings.TrimRight(base.Path, "/") + suffix
	base.RawPath = escapedPrefix + suffix
	query := base.Query()
	query.Set(TaskArtifactAccessQueryParameter, access)
	base.RawQuery = query.Encode()
	return base.String(), nil
}
