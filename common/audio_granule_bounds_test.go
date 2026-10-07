package common

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func opusTestPage(granule uint64) []byte {
	data := make([]byte, 27)
	copy(data, "OggS")
	binary.LittleEndian.PutUint64(data[6:14], granule)
	return data
}

func TestOpusGranuleOverflowRejectedInsteadOfZeroDuration(t *testing.T) {
	for _, granule := range []uint64{1 << 63, 1<<63 + 1, ^uint64(0) - 1} {
		_, err := getOpusDuration(bytes.NewReader(opusTestPage(granule)))
		require.Error(t, err)
	}
	data := append(opusTestPage(^uint64(0)), opusTestPage(48000)...)
	duration, err := getOpusDuration(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, float64(1), duration)
}
