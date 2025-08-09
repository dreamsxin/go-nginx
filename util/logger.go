package util

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dreamsxin/go-nginx/config"
)

// 初始化日志系统 - 修改为接受配置参数
func InitLogger(cfg *config.Config) error {
	// 解析日志级别
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var logFile *os.File
	// 创建日志目录
	if cfg.Log.File != "" {
		dir := filepath.Dir(cfg.Log.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		// 打开日志文件
		var err error
		logFile, err = os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
	}

	// 确定输出目标
	var output io.Writer
	switch cfg.Log.Output {
	case "file":
		if logFile != nil {
			output = logFile
		} else {
			output = os.Stdout
		}
	case "both":
		if logFile != nil {
			output = io.MultiWriter(os.Stdout, logFile)
		} else {
			output = os.Stdout
		}
	default:
		output = os.Stdout
	}

	// 创建slog处理器
	logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}))

	// 设置全局日志器
	slog.SetDefault(logger)
	return nil
}
