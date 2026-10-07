package business

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
)

type GatewayMemoryStats struct {
	Alloc        uint64 `json:"alloc"`
	TotalAlloc   uint64 `json:"total_alloc"`
	Sys          uint64 `json:"sys"`
	NumGC        uint32 `json:"num_gc"`
	NumGoroutine int    `json:"num_goroutine"`
}

type GatewayDiskCacheInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

type GatewayPerformanceConfig struct {
	DiskCacheEnabled       bool   `json:"disk_cache_enabled"`
	DiskCacheThresholdMB   int    `json:"disk_cache_threshold_mb"`
	DiskCacheMaxSizeMB     int    `json:"disk_cache_max_size_mb"`
	DiskCachePath          string `json:"disk_cache_path"`
	IsRunningInContainer   bool   `json:"is_running_in_container"`
	MonitorEnabled         bool   `json:"monitor_enabled"`
	MonitorCPUThreshold    int    `json:"monitor_cpu_threshold"`
	MonitorMemoryThreshold int    `json:"monitor_memory_threshold"`
	MonitorDiskThreshold   int    `json:"monitor_disk_threshold"`
}

type GatewayPerformanceStats struct {
	CacheStats    common.DiskCacheStats    `json:"cache_stats"`
	MemoryStats   GatewayMemoryStats       `json:"memory_stats"`
	DiskCacheInfo GatewayDiskCacheInfo     `json:"disk_cache_info"`
	DiskSpaceInfo common.DiskSpaceInfo     `json:"disk_space_info"`
	Config        GatewayPerformanceConfig `json:"config"`
}

func GatewayCacheDirectorySnapshot(stats common.DiskCacheStats) GatewayDiskCacheInfo {
	path := common.GetDiskCacheDir()
	info := GatewayDiskCacheInfo{Path: path, FileCount: int(stats.ActiveDiskFiles), TotalSize: stats.CurrentDiskUsageBytes}
	directory, err := os.Stat(path)
	info.Exists = err == nil && directory.IsDir()
	return info
}

func ReadGatewayPerformanceContext(ctx context.Context) (*GatewayPerformanceStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache := common.GetDiskCacheStats()
	disk, monitor := common.GetDiskCacheConfig(), common.GetPerformanceMonitorConfig()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result := &GatewayPerformanceStats{
		CacheStats:    cache,
		MemoryStats:   GatewayMemoryStats{Alloc: memory.Alloc, TotalAlloc: memory.TotalAlloc, Sys: memory.Sys, NumGC: memory.NumGC, NumGoroutine: runtime.NumGoroutine()},
		DiskCacheInfo: GatewayCacheDirectorySnapshot(cache),
		DiskSpaceInfo: common.GetDiskSpaceInfo(),
		Config:        GatewayPerformanceConfig{DiskCacheEnabled: disk.Enabled, DiskCacheThresholdMB: disk.ThresholdMB, DiskCacheMaxSizeMB: disk.MaxSizeMB, DiskCachePath: disk.Path, IsRunningInContainer: common.IsRunningInContainer(), MonitorEnabled: monitor.Enabled, MonitorCPUThreshold: monitor.CPUThreshold, MonitorMemoryThreshold: monitor.MemoryThreshold, MonitorDiskThreshold: monitor.DiskThreshold},
	}
	return result, ctx.Err()
}

type GatewayLogFile struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type GatewayLogDirectory struct {
	LogDir     string           `json:"log_dir"`
	Enabled    bool             `json:"enabled"`
	FileCount  int              `json:"file_count"`
	TotalSize  int64            `json:"total_size"`
	OldestTime *time.Time       `json:"oldest_time,omitempty"`
	NewestTime *time.Time       `json:"newest_time,omitempty"`
	Files      []GatewayLogFile `json:"files"`
}

func gatewayLogDirectory() string {
	if common.LogDir == nil {
		return ""
	}
	return strings.TrimSpace(*common.LogDir)
}
func gatewayLogFileName(name string) bool {
	return strings.HasPrefix(name, "further-api-") && strings.HasSuffix(name, ".log")
}

func readGatewayLogFiles(ctx context.Context, path string) ([]GatewayLogFile, error) {
	files := []GatewayLogFile{}
	directory, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return files, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := directory.ReadDir(128)
		for _, entry := range entries {
			if !gatewayLogFileName(entry.Name()) || !entry.Type().IsRegular() {
				continue
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			files = append(files, GatewayLogFile{Name: entry.Name(), Size: info.Size(), ModTime: info.ModTime()})
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files, nil
}

func ReadGatewayLogDirectoryContext(ctx context.Context) (*GatewayLogDirectory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &GatewayLogDirectory{LogDir: gatewayLogDirectory(), Files: []GatewayLogFile{}}
	if result.LogDir == "" {
		return result, nil
	}
	result.Enabled = true
	files, err := readGatewayLogFiles(ctx, result.LogDir)
	if err != nil {
		return nil, err
	}
	result.Files, result.FileCount = files, len(files)
	for _, file := range files {
		result.TotalSize += file.Size
		if result.OldestTime == nil || file.ModTime.Before(*result.OldestTime) {
			stamp := file.ModTime
			result.OldestTime = &stamp
		}
		if result.NewestTime == nil || file.ModTime.After(*result.NewestTime) {
			stamp := file.ModTime
			result.NewestTime = &stamp
		}
	}
	return result, nil
}

type GatewayLogRetentionInput struct {
	Mode  string
	Value int
}
type GatewayLogRetentionResult struct {
	DeletedCount int      `json:"deleted_count"`
	FreedBytes   int64    `json:"freed_bytes"`
	FailedFiles  []string `json:"failed_files"`
}

func ApplyGatewayLogRetentionContext(ctx context.Context, input GatewayLogRetentionInput) (*GatewayLogRetentionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.Mode != "by_count" && input.Mode != "by_days" {
		return nil, fmt.Errorf("请选择按文件数量或保留天数清理")
	}
	if input.Value < 1 {
		return nil, fmt.Errorf("保留数量或天数须为正整数")
	}
	if input.Mode == "by_days" && int64(input.Value) > math.MaxInt64/int64(24*time.Hour) {
		return nil, fmt.Errorf("保留天数超出有效范围")
	}
	path := gatewayLogDirectory()
	if path == "" {
		return nil, fmt.Errorf("尚未设置日志目录")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	files, err := readGatewayLogFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	result := &GatewayLogRetentionResult{FailedFiles: []string{}}
	cutoff := time.Now()
	if input.Mode == "by_days" {
		cutoff = cutoff.Add(-time.Duration(input.Value) * 24 * time.Hour)
	}
	for index, file := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if input.Mode == "by_count" && index < input.Value {
			continue
		}
		if input.Mode == "by_days" && !file.ModTime.Before(cutoff) {
			continue
		}
		path := filepath.Join(root, file.Name)
		current, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			result.FailedFiles = append(result.FailedFiles, file.Name)
			continue
		}
		if !current.Mode().IsRegular() || !current.ModTime().Equal(file.ModTime) {
			continue
		}
		deleted, err := logger.RemoveInactiveLogFile(path)
		if err != nil {
			result.FailedFiles = append(result.FailedFiles, file.Name)
			continue
		}
		if deleted {
			result.DeletedCount++
			result.FreedBytes += current.Size()
		}
	}
	if len(result.FailedFiles) > 0 {
		return result, fmt.Errorf("%d 个日志文件清理失败，请稍后重试", len(result.FailedFiles))
	}
	return result, nil
}
