package service

import (
	"bytes"
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInlineFilesWithEqualPrefixesAndLengthsKeepTheirOwnContents(t *testing.T) {
	shared := bytes.Repeat([]byte{0}, 100)
	first := base64.StdEncoding.EncodeToString(append(append([]byte(nil), shared...), bytes.Repeat([]byte{'A'}, 100)...))
	second := base64.StdEncoding.EncodeToString(append(append([]byte(nil), shared...), bytes.Repeat([]byte{'B'}, 100)...))
	require.Equal(t, len(first), len(second))
	require.Equal(t, first[:128], second[:128], "these payloads collided with the old partial cache identity")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	defer CleanupFileSources(c)
	for _, expected := range []string{first, second} {
		cached, err := LoadFileSource(c, types.NewBase64FileSource(expected, "application/octet-stream"))
		require.NoError(t, err)
		actual, err := cached.GetBase64Data()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}
