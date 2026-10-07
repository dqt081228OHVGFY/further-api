package logger

import (
	"os"
	"path/filepath"
	"strings"
)

// Rotation and retention share the admission lock, so the active output cannot
// become a deletion candidate between checking its name and removing a file.
func RemoveInactiveLogFile(path string) (bool, error) {
	setupLogLock.Lock()
	defer setupLogLock.Unlock()
	currentLogPathMu.RLock()
	active := currentLogPath
	currentLogPathMu.RUnlock()
	resolved, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	if active != "" {
		current, err := filepath.Abs(active)
		if err != nil {
			return false, err
		}
		if strings.EqualFold(filepath.Clean(current), filepath.Clean(resolved)) {
			return false, nil
		}
	}
	if err := os.Remove(resolved); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
