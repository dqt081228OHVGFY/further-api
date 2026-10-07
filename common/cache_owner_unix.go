//go:build unix

package common

import (
	"fmt"
	"os"
	"syscall"
)

func validateCacheDirectoryOwner(info os.FileInfo) error {
	stat, valid := info.Sys().(*syscall.Stat_t)
	if !valid || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("cache directory must belong to the gateway process user")
	}
	return nil
}
