package common

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/stretchr/testify/require"
)

func testPayloadLimit(t *testing.T, limit int) {
	t.Helper()
	previous := constant.ExternalPayloadMemoryBudgetMB
	baseline := PayloadMemoryReservedBytes()
	constant.ExternalPayloadMemoryBudgetMB = limit
	t.Cleanup(func() {
		constant.ExternalPayloadMemoryBudgetMB = previous
		require.Equal(t, baseline, PayloadMemoryReservedBytes(), "capacity must be returned after every exit")
	})
}

func TestPayloadAdmissionIsAtomicAndReservationsAreIdempotent(t *testing.T) {
	testPayloadLimit(t, 24)
	var mutex sync.Mutex
	var reservations []*PayloadMemoryReservation
	var workers sync.WaitGroup
	start := make(chan struct{})
	for index := 0; index < 32; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			reservation, err := ReservePayloadMemory(context.Background(), 12<<20)
			if err == nil {
				mutex.Lock()
				reservations = append(reservations, reservation)
				mutex.Unlock()
			}
		}()
	}
	close(start)
	workers.Wait()
	require.Len(t, reservations, 2)
	require.Equal(t, int64(24<<20), PayloadMemoryReservedBytes())
	for _, reservation := range reservations {
		reservation.Release()
		reservation.Release()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReservePayloadMemory(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestBodyCapacityRemainsReservedUntilOutstandingReaderCloses(t *testing.T) {
	testPayloadLimit(t, 8)
	data := bytes.Repeat([]byte("a"), 256<<10)
	storage, err := CreateBodyStorageFromReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), int64(len(data)))
	require.NoError(t, err)
	reader, err := storage.NewReader()
	require.NoError(t, err)
	require.NoError(t, storage.Close())
	require.Positive(t, PayloadMemoryReservedBytes())
	read, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data, read)
	require.NoError(t, reader.Close())
	require.NoError(t, reader.Close())
}

func TestUnknownBodyAndJSONAllocationBoundsReleaseCapacity(t *testing.T) {
	testPayloadLimit(t, 1)
	_, err := CreateBodyStorageFromReaderContext(context.Background(), strings.NewReader(strings.Repeat("x", 512<<10)), -1, 1<<20)
	require.ErrorIs(t, err, ErrPayloadMemoryCapacity)
	require.Zero(t, PayloadMemoryReservedBytes())
	previousDepth, previousNodes := constant.RequestJSONMaxDepth, constant.RequestJSONMaxNodes
	t.Cleanup(func() { constant.RequestJSONMaxDepth, constant.RequestJSONMaxNodes = previousDepth, previousNodes })
	constant.RequestJSONMaxDepth, constant.RequestJSONMaxNodes = 8, 20
	for _, data := range []string{strings.Repeat("[", 12) + "0" + strings.Repeat("]", 12), "[" + strings.Repeat("0,", 30) + "0]"} {
		storage, err := CreateBodyStorage([]byte(data))
		require.NoError(t, err)
		require.ErrorIs(t, ValidateRequestJSONStructure(context.Background(), storage), ErrRequestBodyTooLarge)
		require.NoError(t, storage.Close())
	}
	data := `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`
	constant.RequestJSONMaxNodes = 100
	storage, err := CreateBodyStorage([]byte(data))
	require.NoError(t, err)
	require.NoError(t, ValidateRequestJSONStructure(context.Background(), storage))
	require.NoError(t, storage.Close())
}

func TestCacheDirectoryRejectsSymlink(t *testing.T) {
	parent := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(parent, diskCacheDir)); err != nil {
		t.Skip("platform does not permit symlink creation")
	}
	previous := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previous) })
	configured := previous
	configured.Path = parent
	SetDiskCacheConfig(configured)
	require.Error(t, EnsureDiskCacheDir())
	_, _, err := CreateDiskCacheFile(DiskCacheTypeBody)
	require.Error(t, err)
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	require.Empty(t, entries, "symlink destination must not receive a cache file")
}
