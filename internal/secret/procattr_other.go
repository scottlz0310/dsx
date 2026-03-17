//go:build !windows

package secret

import "syscall"

// detachProcAttr は非 Windows 向けのプロセス属性を返します。
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
