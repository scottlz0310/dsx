//go:build windows

package secret

import "syscall"

// detachProcAttr は Windows 向けのプロセス属性を返します。
// CREATE_NEW_PROCESS_GROUP により、デタッチ起動したプロセスが
// 親プロセスのコンソールシグナル（Ctrl+C 等）の影響を受けないようにします。
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
