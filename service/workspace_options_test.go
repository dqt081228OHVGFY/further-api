package service

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func workspaceTestOptions(t *testing.T, values map[string]string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previousMap, previousSecret := common.OptionMap, common.LinuxDOClientSecret
	common.OptionMap, common.LinuxDOClientSecret = values, "private-linuxdo-credential"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap, common.LinuxDOClientSecret = previousMap, previousSecret
		common.OptionMapRWMutex.Unlock()
	})
}

func TestWorkspaceOptionsFilterCredentialsAndRetiredFeatures(t *testing.T) {
	workspaceTestOptions(t, map[string]string{
		"SMTPToken": "private-smtp", "smtp.password": "private-password",
		"discord.client_secret": "private-discord", "JWT_PRIVATE_KEY_PEM": "private-signing-key",
		"service.API_KEY": "private-api-key", "service.AccessToken": "private-access-token",
		"service.credential": "private-credential", "Chats": "retired-chat-presets",
		"theme.frontend": "retired-frontend", "SystemName": common.ProductName,
		"billing_setting.billing_mode": "stale-mode", "billing_setting.billing_expr": "stale-expr",
		"ModelRatio": `{"visible-model":1}`, "ModelPrice": `{invalid`,
	})
	options, err := ReadWorkspaceOptions()
	require.NoError(t, err)
	encoded, err := json.Marshal(options)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
	require.NotContains(t, string(encoded), "retired-")
	require.NotContains(t, string(encoded), "stale-")
	values := make(map[string]string)
	keys := make([]string, 0, len(options))
	for _, option := range options {
		values[option.Key] = option.Value
		keys = append(keys, option.Key)
	}
	require.True(t, sort.StringsAreSorted(keys))
	require.Equal(t, "true", values["LinuxDOClientSecretConfigured"])
	require.Contains(t, values["CompletionRatioMeta"], "visible-model")
	for _, key := range []string{"billing_setting.billing_mode", "billing_setting.billing_expr"} {
		require.True(t, json.Valid([]byte(values[key])))
	}
	again, err := ReadWorkspaceOptions()
	require.NoError(t, err)
	require.Equal(t, options, again)
	options[0].Value = "caller-mutation"
	again, err = ReadWorkspaceOptions()
	require.NoError(t, err)
	require.NotEqual(t, "caller-mutation", again[0].Value)
}

func TestWorkspaceOptionsExposeCredentialStateWithoutCredentialValue(t *testing.T) {
	workspaceTestOptions(t, map[string]string{
		"SMTPToken": "smtp-sensitive-value", "SMTPTokenConfigured": "false",
		"discord.client_secret": "", "ordinary_setting": "visible",
	})
	options, err := ReadWorkspaceOptions()
	require.NoError(t, err)
	values := make(map[string]string)
	for _, option := range options {
		values[option.Key] = option.Value
	}
	require.Equal(t, "true", values["SMTPTokenConfigured"])
	require.Equal(t, "false", values["discord.client_secretConfigured"])
	require.NotContains(t, values, "SMTPToken")
	require.NotContains(t, values, "discord.client_secret")
	require.Equal(t, "visible", values["ordinary_setting"])
	encoded, err := json.Marshal(options)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "smtp-sensitive-value")
}

func TestWorkspaceOptionsReadDoesNotRequireExclusiveLock(t *testing.T) {
	workspaceTestOptions(t, map[string]string{"SystemName": common.ProductName})
	common.OptionMapRWMutex.RLock()
	finished := make(chan error, 1)
	go func() {
		_, err := ReadWorkspaceOptions()
		finished <- err
	}()
	select {
	case err := <-finished:
		common.OptionMapRWMutex.RUnlock()
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		common.OptionMapRWMutex.RUnlock()
		<-finished
		t.Fatal("reading workspace options blocked another reader")
	}
}
