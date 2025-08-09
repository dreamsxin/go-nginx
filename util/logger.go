package util

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/dreamsxin/go-nginx/config"
)

var (
	logger   *log.Logger
	logFile  *os.File
	logLevel string
	mu       sync.Mutex
)

// 日志级别常量
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// 初始化日志系统
func InitLogger() error {
	mu.Lock()
	defer mu.Unlock()

	// 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		return err
	}

	logLevel = cfg.Log.Level

	// 创建日志目录
	if cfg.Log.File != "" {
		dir := filepath.Dir(cfg.Log.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		// 打开日志文件
		logFile, err = os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
	}

	// 设置输出目标
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

	// 创建 logger 实例
	logger = log.New(output, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	return nil
}

// 调试级别日志
func LogDebugf(format string, v ...interface{}) {
	if logLevel == string(LogLevelDebug) {
		mu.Lock()
		defer mu.Unlock()
		logger.Printf("[DEBUG] "+format, v...)
	}
}

// 信息级别日志
func LogInfof(format string, v ...interface{}) {
	if logLevel == string(LogLevelDebug) || logLevel == string(LogLevelInfo) {
		mu.Lock()
		defer mu.Unlock()
		logger.Printf("[INFO] "+format, v...)
	}
}

// 警告级别日志
func LogWarnf(format string, v ...interface{}) {
	if logLevel == string(LogLevelDebug) || logLevel == string(LogLevelInfo) || logLevel == string(LogLevelWarn) {
		mu.Lock()
		defer mu.Unlock()
		logger.Printf("[WARN] "+format, v...)
	}
}

// 错误级别日志
func LogErrorf(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	logger.Printf("[ERROR] "+format, v...)
}

// 关闭日志系统
func CloseLogger() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
	}
}
