//go:build !unix

package common

import "os"

// Unix ownership/mode checks do not describe Windows ACLs. Windows deployments
// use an operator-controlled cache path and retain the platform directory ACL.
func validateCacheDirectoryOwner(_ os.FileInfo) error { return nil }
