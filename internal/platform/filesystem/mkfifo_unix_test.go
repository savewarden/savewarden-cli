//go:build unix

package filesystem_test

import "syscall"

func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }
