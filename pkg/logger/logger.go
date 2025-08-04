package logger

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
)

// InitLogger 初始化全局Logger (线程安全)
func InitLogger() {
	once.Do(func() {
		config := zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 可读时间格式

		var err error
		globalLogger, err = config.Build(
			zap.AddCaller(),                   // 记录调用位置
			zap.AddStacktrace(zap.ErrorLevel), // 错误级别记录调用栈
		)
		if err != nil {
			panic("初始化Logger失败: " + err.Error())
		}
	})
}

// L 获取全局Logger实例
func L() *zap.Logger {
	if globalLogger == nil {
		InitLogger() // 懒加载初始化
	}
	return globalLogger
}

// Sync 刷新日志缓冲（主函数退出前调用）
func Sync() error {
	return L().Sync()
}
