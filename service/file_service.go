package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/webp"
)

// FileService 统一的文件处理服务
// 提供文件下载、解码、缓存等功能的统一入口

// getContextCacheKey 生成 URL context 缓存的 key
func getContextCacheKey(url string) string {
	return fmt.Sprintf("file_cache_%s", common.GenerateHMAC(url))
}

// getBase64ContextCacheKey 生成 base64 context 缓存的 key
// Use the complete payload identity: equal lengths and matching prefixes can
// still describe different images, dimensions, or documents in one request.
func getBase64ContextCacheKey(data string, mimeType string) string {
	digest := sha256.Sum256([]byte(data))
	keyMaterial := fmt.Sprintf("%d:%s:%x", len(data), mimeType, digest)
	return fmt.Sprintf("b64_cache_%s", common.GenerateHMAC(keyMaterial))
}

// LoadFileSource 加载文件源数据
// 这是统一的入口，会自动处理缓存和不同的来源类型
func LoadFileSource(c *gin.Context, source types.FileSource, reason ...string) (*types.CachedFileData, error) {
	if source == nil {
		return nil, fmt.Errorf("file source is nil")
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "LoadFileSource starting for: %s", source.GetIdentifier())
	}

	// 1. 快速检查内部缓存
	if source.HasCache() {
		if c != nil {
			registerSourceForCleanup(c, source)
		}
		return source.GetCache(), nil
	}

	// 2. 加锁保护加载过程
	source.Mu().Lock()
	defer source.Mu().Unlock()

	// 3. 双重检查
	if source.HasCache() {
		if c != nil {
			registerSourceForCleanup(c, source)
		}
		return source.GetCache(), nil
	}

	// 4. 根据来源类型加载（含 URL context 缓存查找）
	var cachedData *types.CachedFileData
	var contextKey string
	var err error

	switch s := source.(type) {
	case *types.URLSource:
		if c != nil {
			contextKey = getContextCacheKey(s.URL)
			if cached, exists := c.Get(contextKey); exists {
				data := cached.(*types.CachedFileData)
				source.SetCache(data)
				registerSourceForCleanup(c, source)
				return data, nil
			}
		}
		cachedData, err = loadFromURL(c, s.URL, reason...)
	case *types.Base64Source:
		if c != nil {
			contextKey = getBase64ContextCacheKey(s.Base64Data, s.MimeType)
			if cached, exists := c.Get(contextKey); exists {
				data := cached.(*types.CachedFileData)
				source.SetCache(data)
				registerSourceForCleanup(c, source)
				return data, nil
			}
		}
		cachedData, err = loadBase64ForRequest(c, s.Base64Data, s.MimeType)
	default:
		return nil, fmt.Errorf("unsupported file source type: %T", source)
	}

	if err != nil {
		return nil, err
	}

	// 5. 设置缓存
	source.SetCache(cachedData)
	if contextKey != "" && c != nil {
		c.Set(contextKey, cachedData)
	}

	// 6. 注册到 context 以便请求结束时自动清理
	if c != nil {
		registerSourceForCleanup(c, source)
	}

	return cachedData, nil
}

// registerSourceForCleanup 注册 FileSource 到 context 以便请求结束时清理
func registerSourceForCleanup(c *gin.Context, source types.FileSource) {
	if source.IsRegistered() {
		return
	}

	key := string(constant.ContextKeyFileSourcesToCleanup)
	var sources []types.FileSource
	if existing, exists := c.Get(key); exists {
		sources = existing.([]types.FileSource)
	}
	sources = append(sources, source)
	c.Set(key, sources)
	source.SetRegistered(true)
}

// CleanupFileSources 清理请求中所有注册的 FileSource
// 应在请求结束时调用（通常由中间件自动调用）
func CleanupFileSources(c *gin.Context) {
	key := string(constant.ContextKeyFileSourcesToCleanup)
	if sources, exists := c.Get(key); exists {
		for _, source := range sources.([]types.FileSource) {
			if cache := source.GetCache(); cache != nil {
				cache.Close()
			}
		}
		c.Set(key, nil)
	}
}

// loadFromURL 从 URL 加载文件
func loadFromURL(c *gin.Context, url string, reason ...string) (*types.CachedFileData, error) {
	return streamFileFromURL(c, url, reason...)
}

// shouldUseDiskCache 判断是否应该使用磁盘缓存
func shouldUseDiskCache(dataSize int64) bool {
	return common.ShouldUseDiskCache(dataSize)
}

// writeToDiskCache 将数据写入磁盘缓存
func writeToDiskCache(base64Data string) (string, error) {
	return common.WriteDiskCacheFileString(common.DiskCacheTypeFile, base64Data)
}

// smartDetectMimeType 智能检测 MIME 类型
func smartDetectMimeType(resp *http.Response, url string, fileBytes []byte) string {
	// 1. 尝试从 Content-Type header 获取
	mimeType := resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType
	}

	// 2. 尝试从 Content-Disposition header 的 filename 获取
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		parts := strings.SplitSeq(cd, ";")
		for part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToLower(part), "filename=") {
				name := strings.TrimSpace(strings.TrimPrefix(part, "filename="))
				// 移除引号
				if len(name) > 2 && name[0] == '"' && name[len(name)-1] == '"' {
					name = name[1 : len(name)-1]
				}
				if dot := strings.LastIndex(name, "."); dot != -1 && dot+1 < len(name) {
					ext := strings.ToLower(name[dot+1:])
					if ext != "" {
						mt := GetMimeTypeByExtension(ext)
						if mt != "application/octet-stream" {
							return mt
						}
					}
				}
				break
			}
		}
	}

	// 3. 尝试从 URL 路径获取扩展名
	mt := guessMimeTypeFromURL(url)
	if mt != "application/octet-stream" {
		return mt
	}

	// 4. 使用 http.DetectContentType 内容嗅探
	if len(fileBytes) > 0 {
		sniffed := http.DetectContentType(fileBytes)
		if sniffed != "" && sniffed != "application/octet-stream" {
			// 去除可能的 charset 参数
			if idx := strings.Index(sniffed, ";"); idx != -1 {
				sniffed = strings.TrimSpace(sniffed[:idx])
			}
			return sniffed
		}

		// 4.5 尝试 HEIF/HEIC 检测（Go 标准库不识别）
		if heifMime := detectHEIF(fileBytes); heifMime != "" {
			return heifMime
		}
	}

	// 5. 尝试作为图片解码获取格式
	if len(fileBytes) > 0 {
		if _, format, err := decodeImageConfig(fileBytes); err == nil && format != "" {
			return "image/" + strings.ToLower(format)
		}
	}

	// 最终回退
	return "application/octet-stream"
}

// loadFromBase64 从 base64 字符串加载文件
func loadFromBase64(data string, mimeType string) (*types.CachedFileData, error) {
	return loadBase64ForRequest(nil, data, mimeType)
}

// GetImageConfig 获取图片配置
func GetImageConfig(c *gin.Context, source types.FileSource) (image.Config, string, error) {
	cachedData, err := LoadFileSource(c, source, "get_image_config")
	if err != nil {
		return image.Config{}, "", err
	}

	if cachedData.ImageConfig != nil {
		return *cachedData.ImageConfig, cachedData.ImageFormat, nil
	}

	reader, err := cachedData.OpenBase64Reader()
	if err != nil {
		return image.Config{}, "", fmt.Errorf("failed to get base64 data: %w", err)
	}
	defer reader.Close()
	config, format, err := decodeImageConfigReader(base64.NewDecoder(base64.StdEncoding, reader))
	if err != nil {
		return image.Config{}, "", err
	}

	cachedData.ImageConfig = &config
	cachedData.ImageFormat = format

	return config, format, nil
}

// GetBase64Data 获取 base64 编码的数据
func GetBase64Data(c *gin.Context, source types.FileSource, reason ...string) (string, string, error) {
	cachedData, err := LoadFileSource(c, source, reason...)
	if err != nil {
		return "", "", err
	}
	base64Str, err := cachedData.GetBase64Data()
	if err != nil {
		return "", "", fmt.Errorf("failed to get base64 data: %w", err)
	}
	return base64Str, cachedData.MimeType, nil
}

// GetMimeType 获取文件的 MIME 类型
func GetMimeType(c *gin.Context, source types.FileSource) (string, error) {
	if source.HasCache() {
		return source.GetCache().MimeType, nil
	}

	if urlSource, ok := source.(*types.URLSource); ok {
		mimeType, err := GetFileTypeFromUrl(c, urlSource.URL, "get_mime_type")
		if err == nil && mimeType != "" && mimeType != "application/octet-stream" {
			return mimeType, nil
		}
	}

	cachedData, err := LoadFileSource(c, source, "get_mime_type")
	if err != nil {
		return "", err
	}
	return cachedData.MimeType, nil
}

// DetectFileType 检测文件类型
func DetectFileType(mimeType string) types.FileType {
	if strings.HasPrefix(mimeType, "image/") {
		return types.FileTypeImage
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return types.FileTypeAudio
	}
	if strings.HasPrefix(mimeType, "video/") {
		return types.FileTypeVideo
	}
	return types.FileTypeFile
}

// decodeImageConfig 从字节数据解码图片配置
func decodeImageConfig(data []byte) (image.Config, string, error) {
	reader := bytes.NewReader(data)

	config, format, err := image.DecodeConfig(reader)
	if err == nil {
		return config, format, nil
	}

	reader.Seek(0, io.SeekStart)
	config, err = webp.DecodeConfig(reader)
	if err == nil {
		return config, "webp", nil
	}

	// Try HEIF/HEIC: parse ISOBMFF ispe box for dimensions
	if heifMime := detectHEIF(data); heifMime != "" {
		formatName := "heif"
		if heifMime == "image/heic" {
			formatName = "heic"
		}
		if w, h, ok := parseHEIFDimensions(data); ok {
			return image.Config{Width: w, Height: h}, formatName, nil
		}
		return image.Config{}, "", fmt.Errorf("failed to decode HEIF/HEIC image dimensions")
	}

	return image.Config{}, "", fmt.Errorf("failed to decode image config: unsupported format")
}

// detectHEIF checks ISOBMFF magic bytes to detect HEIC/HEIF files.
// Returns "image/heic", "image/heif", or "" if not recognized.
func detectHEIF(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	// ISOBMFF: bytes[4:8] must be "ftyp"
	if string(data[4:8]) != "ftyp" {
		return ""
	}
	brand := string(data[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis":
		return "image/heic"
	case "mif1", "msf1":
		return "image/heif"
	default:
		return ""
	}
}

// parseHEIFDimensions parses ISOBMFF box tree to find the ispe box
// and extract image width/height. Returns (width, height, ok).
func parseHEIFDimensions(data []byte) (int, int, bool) {
	size := len(data)
	if size < 12 {
		return 0, 0, false
	}

	// Walk top-level boxes to find "meta"
	offset := 0
	for size-offset >= 8 {
		boxSize, headerLen, boxType, valid := readHEIFBox(data[offset:])
		if !valid {
			return 0, 0, false
		}

		if boxType == "meta" {
			// meta is a full box: 4 bytes version/flags after header
			metaData := data[offset+headerLen : offset+boxSize]
			if len(metaData) < 4 {
				return 0, 0, false
			}
			return findISPE(metaData[4:])
		}
		offset += boxSize
	}
	return 0, 0, false
}

// findISPE recursively searches for the ispe box within container boxes.
// Path: meta -> iprp -> ipco -> ispe
func findISPE(data []byte) (int, int, bool) {
	return findISPEAtDepth(data, 0)
}

// Validate the unsigned size against available bytes before converting to int
// or adding an offset. Both regular and extended boxes share this boundary.
func readHEIFBox(data []byte) (size, header int, kind string, valid bool) {
	if len(data) < 8 {
		return 0, 0, "", false
	}
	length := uint64(binary.BigEndian.Uint32(data[:4]))
	header = 8
	if length == 1 {
		if len(data) < 16 {
			return 0, 0, "", false
		}
		length = binary.BigEndian.Uint64(data[8:16])
		header = 16
	} else if length == 0 {
		length = uint64(len(data))
	}
	if length < uint64(header) || length > uint64(len(data)) {
		return 0, 0, "", false
	}
	return int(length), header, string(data[4:8]), true
}

func findISPEAtDepth(data []byte, depth int) (int, int, bool) {
	if depth > 32 {
		return 0, 0, false
	}
	offset := 0
	size := len(data)
	for size-offset >= 8 {
		boxSize, headerLen, boxType, valid := readHEIFBox(data[offset:])
		if !valid {
			return 0, 0, false
		}
		content := data[offset+headerLen : offset+boxSize]
		switch boxType {
		case "iprp", "ipco":
			if w, h, ok := findISPEAtDepth(content, depth+1); ok {
				return w, h, true
			}
		case "ispe":
			// ispe is a full box: 4 bytes version/flags, then 4 bytes width, 4 bytes height
			if len(content) >= 12 {
				w := uint64(binary.BigEndian.Uint32(content[4:8]))
				h := uint64(binary.BigEndian.Uint32(content[8:12]))
				maxInt := uint64(^uint(0) >> 1)
				if w > 0 && h > 0 && w <= maxInt && h <= maxInt {
					return int(w), int(h), true
				}
			}
		}
		offset += boxSize
	}
	return 0, 0, false
}

// guessMimeTypeFromURL 从 URL 猜测 MIME 类型
func guessMimeTypeFromURL(url string) string {
	cleanedURL := url
	if q := strings.Index(cleanedURL, "?"); q != -1 {
		cleanedURL = cleanedURL[:q]
	}

	if slash := strings.LastIndex(cleanedURL, "/"); slash != -1 && slash+1 < len(cleanedURL) {
		last := cleanedURL[slash+1:]
		if dot := strings.LastIndex(last, "."); dot != -1 && dot+1 < len(last) {
			ext := strings.ToLower(last[dot+1:])
			return GetMimeTypeByExtension(ext)
		}
	}

	return "application/octet-stream"
}
