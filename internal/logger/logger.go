package logger

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化全局 zap logger。
//
// level: debug | info | warn | error
// format: console | json
//
// 初始化后可通过 zap.L() 和 zap.S() 全局访问。
func Init(level, format string) error {
	zapLevel, err := parseLevel(level)
	if err != nil {
		return err
	}

	var cfg zap.Config
	switch strings.ToLower(format) {
	case "json":
		cfg = zap.NewProductionConfig()
	default: // "console" 或其他
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	// 统一时间格式
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build(
		zap.AddCallerSkip(0),
		zap.AddStacktrace(zapcore.ErrorLevel), // 仅 error 及以上打印堆栈
	)
	if err != nil {
		return fmt.Errorf("build zap logger: %w", err)
	}

	// 替换全局 logger
	zap.ReplaceGlobals(logger)
	return nil
}

// Sync 刷新日志缓冲区，应在程序退出前（defer）调用。
//
// 错误处理策略：
//   - EBADF / EINVAL / "invalid argument"：进程退出时 /dev/stderr 已关闭，属预期行为，静默忽略。
//   - 其他错误（如磁盘满、文件被删除导致 flush 失败）：通过 fmt.Fprintf(os.Stderr) 兜底输出。
//     此时 zap 全局 logger 本身可能已不可用，必须走 os.Stderr 直写。
func Sync() {
	if err := zap.L().Sync(); err != nil {
		if isExpectedSyncError(err) {
			return
		}
		// 真实 flush 失败：日志缓冲区可能未写完，用 stderr 兜底告警
		fmt.Fprintf(os.Stderr, "[logger] Sync failed, buffered logs may be lost: %v\n", err)
	}
}

// isExpectedSyncError 判断是否为 shutdown 阶段的预期错误（无需处理）。
// zap 已知问题：进程退出时 /dev/stderr 关闭后调用 Sync 会返回 EBADF 或 EINVAL。
// 参见：https://github.com/uber-go/zap/issues/772
func isExpectedSyncError(err error) bool {
	// syscall 层面的 EBADF / EINVAL
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EBADF || errno == syscall.EINVAL
	}
	// 部分平台以字符串形式返回
	msg := err.Error()
	return strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "bad file descriptor")
}

// parseLevel 将字符串日志级别转换为 zapcore.Level。
func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown log level %q, falling back to info", level)
	}
}
