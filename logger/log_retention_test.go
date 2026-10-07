package logger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogRetentionCannotRemoveCurrentOutput(t *testing.T) {
	directory := t.TempDir()
	active := filepath.Join(directory, "further-api-active.log")
	inactive := filepath.Join(directory, "further-api-closed.log")
	for _, path := range []string{active, inactive} {
		require.NoError(t, os.WriteFile(path, []byte("log"), 0600))
	}
	currentLogPathMu.Lock()
	previous := currentLogPath
	currentLogPath = active
	currentLogPathMu.Unlock()
	t.Cleanup(func() { currentLogPathMu.Lock(); currentLogPath = previous; currentLogPathMu.Unlock() })
	deleted, err := RemoveInactiveLogFile(active)
	require.NoError(t, err)
	require.False(t, deleted)
	_, err = os.Stat(active)
	require.NoError(t, err)
	deleted, err = RemoveInactiveLogFile(inactive)
	require.NoError(t, err)
	require.True(t, deleted)
	_, err = os.Stat(inactive)
	require.True(t, os.IsNotExist(err))
}
