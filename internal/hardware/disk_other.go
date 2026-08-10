//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !windows

package hardware

func freeDiskMB(path string) uint64 {
	return 0
}
