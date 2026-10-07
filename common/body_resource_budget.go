package common

import (
	"context"
	"fmt"
	"io"
	"sync"
)

const bodyMemoryOverhead = 64 << 10

func bodyMemoryWeight(size int64) int64 {
	if size < 0 || size > ((1<<62)-bodyMemoryOverhead)/4 {
		return 1 << 62
	}
	return size*4 + bodyMemoryOverhead
}

type budgetedBodyStorage struct {
	BodyStorage
	reservation *PayloadMemoryReservation
	mutex       sync.Mutex
	closed      bool
	readers     int
	nodes       int64
}

func wrapBudgetedBody(storage BodyStorage, reservation *PayloadMemoryReservation) BodyStorage {
	return &budgetedBodyStorage{BodyStorage: storage, reservation: reservation}
}

func (storage *budgetedBodyStorage) Close() error {
	storage.mutex.Lock()
	defer storage.mutex.Unlock()
	if storage.closed {
		return nil
	}
	storage.closed = true
	err := storage.BodyStorage.Close()
	if storage.readers == 0 {
		storage.reservation.Release()
	}
	return err
}

type budgetedBodyReader struct {
	io.ReadCloser
	owner *budgetedBodyStorage
	once  sync.Once
}

func (reader *budgetedBodyReader) Close() error {
	err := reader.ReadCloser.Close()
	reader.once.Do(func() {
		reader.owner.mutex.Lock()
		defer reader.owner.mutex.Unlock()
		reader.owner.readers--
		if reader.owner.closed && reader.owner.readers == 0 {
			reader.owner.reservation.Release()
		}
	})
	return err
}

func (storage *budgetedBodyStorage) NewReader() (io.ReadCloser, error) {
	storage.mutex.Lock()
	defer storage.mutex.Unlock()
	if storage.closed {
		return nil, ErrStorageClosed
	}
	reader, err := storage.BodyStorage.NewReader()
	if err != nil {
		return nil, err
	}
	storage.readers++
	return &budgetedBodyReader{ReadCloser: reader, owner: storage}, nil
}

func (storage *budgetedBodyStorage) reserveJSONNodes(nodes int64) error {
	storage.mutex.Lock()
	defer storage.mutex.Unlock()
	if storage.closed {
		return ErrStorageClosed
	}
	if nodes <= storage.nodes {
		return nil
	}
	if nodes > (1<<62)/512 {
		return ErrPayloadMemoryCapacity
	}
	if err := storage.reservation.Resize(bodyMemoryWeight(storage.Size()) + nodes*512); err != nil {
		return err
	}
	storage.nodes = nodes
	return nil
}

func createBudgetedBodyStorage(data []byte) (BodyStorage, error) {
	reservation, err := ReservePayloadMemory(context.Background(), bodyMemoryWeight(int64(len(data))))
	if err != nil {
		return nil, err
	}
	storage, err := createBodyStorageUnbudgeted(data)
	if err != nil {
		reservation.Release()
		return nil, err
	}
	return wrapBudgetedBody(storage, reservation), nil
}

type bodyGrowingReservationReader struct {
	reader        io.Reader
	context       context.Context
	reservation   *PayloadMemoryReservation
	read, minimum int64
}

func (reader *bodyGrowingReservationReader) Read(data []byte) (int, error) {
	if err := reader.context.Err(); err != nil {
		return 0, err
	}
	if err := reader.reservation.Resize(bodyMemoryWeight(max(reader.minimum, reader.read+int64(len(data))))); err != nil {
		return 0, err
	}
	n, err := reader.reader.Read(data)
	reader.read += int64(n)
	return n, err
}

func CreateBodyStorageFromReaderContext(ctx context.Context, reader io.Reader, contentLength, maxBytes int64) (BodyStorage, error) {
	if maxBytes < 0 || contentLength > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}
	initial := int64(0)
	if contentLength > 0 {
		initial = contentLength
	}
	reservation, err := ReservePayloadMemory(ctx, bodyMemoryWeight(initial))
	if err != nil {
		return nil, err
	}
	tracked := &bodyGrowingReservationReader{reader: reader, context: ctx, reservation: reservation, minimum: initial}
	storage, err := createBodyStorageFromReaderUnbudgeted(tracked, contentLength, maxBytes)
	if err != nil {
		reservation.Release()
		return nil, err
	}
	if err := reservation.Resize(bodyMemoryWeight(storage.Size())); err != nil {
		_ = storage.Close()
		reservation.Release()
		return nil, fmt.Errorf("body allocation reservation failed: %w", err)
	}
	return wrapBudgetedBody(storage, reservation), nil
}
