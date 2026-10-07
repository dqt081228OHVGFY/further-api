package service

import (
	"net/url"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyImageCapabilityUsesDistinctNamespaceAndConfiguredAddress(t *testing.T) {
	previousSecret, previousTaskAddress, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress, system_setting.ServerAddress
	common.CryptoSecret = "image-url-test-secret"
	system_setting.TaskPublicAddress = "https://gateway.example/prefix"
	system_setting.ServerAddress = "https://fallback.example"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		system_setting.TaskPublicAddress = previousTaskAddress
		system_setting.ServerAddress = previousAddress
	})
	raw, err := BuildMidjourneyImageContentURL(42)
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "gateway.example", parsed.Host)
	require.Equal(t, "/prefix/mj/image/42", parsed.Path)
	access := parsed.Query().Get(TaskArtifactAccessQueryParameter)
	require.True(t, VerifyTaskArtifactAccess(access, "midjourney:42", MidjourneyImageArtifactKey))
	require.False(t, VerifyTaskArtifactAccess(access, "42", "image"))
	require.False(t, VerifyTaskArtifactAccess(access, "midjourney:43", "image"))
	for _, id := range []string{"", "0", "-1", "042", "42/../43", " 42"} {
		require.Empty(t, MidjourneyImageAccessTaskID(id))
	}
}
