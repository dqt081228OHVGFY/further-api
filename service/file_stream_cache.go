package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func fileRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}
func maximumFileBytes() int64 { return configuredMB(constant.MaxFileDownloadMB, 64) }

type countFileReader struct {
	io.Reader
	count int64
}

func (reader *countFileReader) Read(data []byte) (int, error) {
	n, err := reader.Reader.Read(data)
	reader.count += int64(n)
	return n, err
}

func streamFileFromURL(c *gin.Context, target string, reason ...string) (_ *types.CachedFileData, err error) {
	ctx := fileRequestContext(c)
	maximum := maximumFileBytes()
	requestBudget := fileBudgetForRequest(c)
	estimate := min(maximum, requestBudget.remaining())
	if estimate <= 0 {
		return nil, ErrRequestFileCapacity
	}
	ticket, err := requestBudget.reserve(estimate)
	if err != nil {
		return nil, err
	}
	counted := &countFileReader{}
	defer func() { ticket.finish(counted.count) }()
	lease, err := externalPayloadResources.acquire(ctx, fileMemoryWeight(estimate), "file")
	if err != nil {
		return nil, err
	}
	kept := false
	defer func() {
		if !kept {
			lease.release()
		}
	}()
	response, err := DoDownloadRequestContext(ctx, target, reason...)
	if err != nil { return nil, fmt.Errorf("failed to download file: %w", err) }
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK { return nil, fmt.Errorf("failed to download file, status code: %d", response.StatusCode) }
	if response.ContentLength > estimate { return nil, fmt.Errorf("file exceeds allocated size limit") }
	counted.Reader = response.Body
	reader := bufio.NewReader(io.LimitReader(counted, min(maximum, estimate)+1))
	header, peekErr := reader.Peek(512)
	if peekErr != nil && peekErr != io.EOF && peekErr != bufio.ErrBufferFull {
		return nil, peekErr
	}
	mediaType := smartDetectMimeType(response, target, header)
	useDisk := common.IsDiskCacheEnabled() && (response.ContentLength < 0 || (estimate+2)/3*4 >= common.GetDiskCacheThresholdBytes())
	var cached *types.CachedFileData
	if useDisk {
		reservedDisk := (estimate + 2) / 3 * 4
		if !common.TryReserveDiskCacheFile(reservedDisk) {
			return nil, fmt.Errorf("file disk cache capacity unavailable")
		}
		path, file, createErr := common.CreateDiskCacheFile(common.DiskCacheTypeFile)
		if createErr != nil {
			common.DecrementDiskFiles(reservedDisk)
			return nil, createErr
		}
		stored := false
		defer func() {
			_ = file.Close()
			if !stored {
				_ = os.Remove(path)
				common.DecrementDiskFiles(reservedDisk)
			}
		}()
		encoder := base64.NewEncoder(base64.StdEncoding, file)
		written, copyErr := io.CopyBuffer(encoder, reader, make([]byte, 32<<10))
		closeErr := encoder.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if written > maximum || written > estimate {
			return nil, fmt.Errorf("file exceeds allocated size limit")
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		actualDisk := (written + 2) / 3 * 4
		common.ReduceDiskCacheReservation(reservedDisk - actualDisk)
		cached = types.NewDiskCachedData(path, mediaType, written)
		cached.DiskSize = actualDisk
		cached.OnClose = common.DecrementDiskFiles
		stored = true
	} else {
		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(data)) > maximum || int64(len(data)) > estimate {
			return nil, fmt.Errorf("file exceeds allocated size limit")
		}
		cached = types.NewMemoryCachedData(base64.StdEncoding.EncodeToString(data), mediaType, int64(len(data)))
	}
	lease.loaded(fileMemoryWeight(cached.Size))
	cached.OnCleanup = lease.release
	populateStreamingImageConfig(cached)
	kept = true
	return cached, nil
}

func splitInlineBase64(data, suppliedType string) (string, string) {
	mediaType := suppliedType
	if strings.HasPrefix(data, "data:") {
		if prefix, value, found := strings.Cut(data, ","); found {
			data = value
			if mediaType == "" {
				if before, _, found := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";"); found {
					mediaType = before
				}
			}
		}
	}
	return data, mediaType
}

func loadBase64ForRequest(c *gin.Context, data, suppliedType string) (_ *types.CachedFileData, err error) {
	clean, mediaType := splitInlineBase64(data, suppliedType)
	estimate := int64(base64.StdEncoding.DecodedLen(len(clean)))
	if estimate > 0 {
		if strings.HasSuffix(clean, "==") {
			estimate -= 2
		} else if strings.HasSuffix(clean, "=") {
			estimate--
		}
	}
	if estimate > maximumFileBytes() {
		return nil, fmt.Errorf("inline file exceeds configured size limit")
	}
	ticket, err := fileBudgetForRequest(c).reserve(max(1, estimate))
	if err != nil {
		return nil, err
	}
	lease, err := externalPayloadResources.acquire(fileRequestContext(c), fileMemoryWeight(estimate), "file")
	if err != nil {
		ticket.finish(0)
		return nil, err
	}
	kept := false
	defer func() {
		if !kept {
			lease.release()
		}
	}()
	written, decodeErr := io.CopyBuffer(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(clean)), make([]byte, 32<<10))
	ticket.finish(written)
	if decodeErr != nil {
		return nil, decodeErr
	}
	var cached *types.CachedFileData
	if shouldUseDiskCache(int64(len(clean))) {
		if !common.TryReserveDiskCacheFile(int64(len(clean))) {
			return nil, fmt.Errorf("file disk cache capacity unavailable")
		}
		path, file, createErr := common.CreateDiskCacheFile(common.DiskCacheTypeFile)
		if createErr != nil {
			common.DecrementDiskFiles(int64(len(clean)))
			return nil, createErr
		}
		_, writeErr := io.CopyBuffer(file, strings.NewReader(clean), make([]byte, 32<<10))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			common.DecrementDiskFiles(int64(len(clean)))
			if writeErr != nil {
				return nil, writeErr
			}
			return nil, closeErr
		}
		cached = types.NewDiskCachedData(path, mediaType, written)
		cached.DiskSize = int64(len(clean))
		cached.OnClose = common.DecrementDiskFiles
	} else {
		cached = types.NewMemoryCachedData(clean, mediaType, written)
	}
	lease.loaded(fileMemoryWeight(written))
	cached.OnCleanup = lease.release
	populateStreamingImageConfig(cached)
	kept = true
	return cached, nil
}

func decodeImageConfigReader(reader io.Reader) (image.Config, string, error) {
	return image.DecodeConfig(reader)
}
func populateStreamingImageConfig(cached *types.CachedFileData) {
	if cached.MimeType != "" && !strings.HasPrefix(cached.MimeType, "image/") {
		return
	}
	reader, err := cached.OpenBase64Reader()
	if err != nil {
		return
	}
	defer reader.Close()
	config, format, err := decodeImageConfigReader(base64.NewDecoder(base64.StdEncoding, reader))
	if err == nil {
		cached.ImageConfig = &config
		cached.ImageFormat = format
		if cached.MimeType == "" {
			cached.MimeType = "image/" + format
		}
	}
}
