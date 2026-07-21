package logger

import (
	"errors"
	"syscall"

	"go.uber.org/multierr"
)

// isBenignSyncError 判定 root logger 的 Sync 失败是否为无害噪音：对终端/管道等
// 字符设备执行 fsync，多数平台返回 EINVAL/ENOTTY/EBADF（console 输出即此类，
// 例如 macOS 上 sync /dev/stderr）。仅当错误链中所有子错误都无害时才判真，
// 避免吞掉文件输出的真实 sync 失败。
func isBenignSyncError(err error) bool {
	for _, e := range multierr.Errors(err) {
		if !errors.Is(e, syscall.EINVAL) && !errors.Is(e, syscall.ENOTTY) && !errors.Is(e, syscall.EBADF) {
			return false
		}
	}
	return true
}
