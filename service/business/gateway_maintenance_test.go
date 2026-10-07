package business

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestGatewayLogRetentionKeepsConfiguredCountAndUnrelatedFiles(t *testing.T) {
	directory := t.TempDir()
	previous := common.LogDir
	common.LogDir = &directory
	t.Cleanup(func() { common.LogDir = previous })
	names := []string{"further-api-20260101000000.log", "further-api-20260201000000.log", "further-api-20260301000000.log", "operator-notes.log"}
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("log"), 0600))
	}
	listing, err := ReadGatewayLogDirectoryContext(context.Background())
	require.NoError(t, err)
	require.Len(t, listing.Files, 3)
	result, err := ApplyGatewayLogRetentionContext(context.Background(), GatewayLogRetentionInput{Mode: "by_count", Value: 1})
	require.NoError(t, err)
	require.Equal(t, 2, result.DeletedCount)
	require.EqualValues(t, 6, result.FreedBytes)
	for _, name := range []string{names[2], names[3]} {
		_, err := os.Stat(filepath.Join(directory, name))
		require.NoError(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ApplyGatewayLogRetentionContext(ctx, GatewayLogRetentionInput{Mode: "by_count", Value: 1})
	require.ErrorIs(t, err, context.Canceled)
}
