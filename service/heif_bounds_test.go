package service

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func heifTestBox(kind string, content []byte) []byte {
	data := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(data[:4], uint32(len(data)))
	copy(data[4:8], kind)
	copy(data[8:], content)
	return data
}

func TestHEIFRejectsExtendedSizeOverflowAtNonzeroOffset(t *testing.T) {
	ftyp := heifTestBox("ftyp", []byte("heic0000"))
	for _, length := range []uint64{math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64, 15, 17} {
		t.Run("invalid_extended_size", func(t *testing.T) {
			meta := make([]byte, 16)
			binary.BigEndian.PutUint32(meta[:4], 1)
			copy(meta[4:8], "meta")
			binary.BigEndian.PutUint64(meta[8:], length)
			data := append(append([]byte{}, ftyp...), meta...)
			require.NotPanics(t, func() {
				_, _, valid := parseHEIFDimensions(data)
				require.False(t, valid)
			})
		})
	}
}

func TestHEIFValidDimensionsAndBoundedContainers(t *testing.T) {
	dimensions := make([]byte, 12)
	binary.BigEndian.PutUint32(dimensions[4:8], 1920)
	binary.BigEndian.PutUint32(dimensions[8:], 1080)
	ispe := heifTestBox("ispe", dimensions)
	meta := heifTestBox("meta", append(make([]byte, 4), heifTestBox("iprp", heifTestBox("ipco", ispe))...))
	w, h, valid := parseHEIFDimensions(append(heifTestBox("ftyp", []byte("heic0000")), meta...))
	require.True(t, valid)
	require.Equal(t, 1920, w)
	require.Equal(t, 1080, h)
	for index := 0; index < 40; index++ {
		ispe = heifTestBox("ipco", ispe)
	}
	_, _, valid = findISPE(ispe)
	require.False(t, valid)
}

func FuzzHEIFBoxBounds(f *testing.F) {
	f.Add([]byte("0000ftypheic0000"))
	f.Add(heifTestBox("meta", make([]byte, 4)))
	f.Fuzz(func(t *testing.T, data []byte) {
		parseHEIFDimensions(data)
		findISPE(data)
	})
}
