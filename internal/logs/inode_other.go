//go:build !unix

package logs

import "os"

func inode(os.FileInfo) uint64 { return 0 }
