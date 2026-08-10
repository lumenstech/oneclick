//go:build linux || darwin || freebsd || openbsd || netbsd

package hardware

import "syscall"

func freeDiskMB(path string) uint64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return st.Bavail * uint64(st.Bsize) / (1024 * 1024)
}
