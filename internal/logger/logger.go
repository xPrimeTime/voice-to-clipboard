package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var (
	// Default logger instance
	defaultLogger *slog.Logger
	logFile       *os.File
)

// Init initializes the logger with file and console output
func Init(logFilePath string, debug bool) error {
	// Ensure log directory exists
	logDir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// Open log file (append mode)
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Multi-writer for both console and file
	multiWriter := io.MultiWriter(os.Stdout, logFile)

	// Set log level based on debug flag
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	// Create handler with appropriate options
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Shorten time format
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   a.Key,
					Value: slog.StringValue(a.Value.Time().Format("15:04:05.000")),
				}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(multiWriter, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)

	return nil
}

// Close closes the log file
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// GetLogger returns the default logger
func GetLogger() *slog.Logger {
	if defaultLogger == nil {
		// Return a default console logger if not initialized
		return slog.Default()
	}
	return defaultLogger
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}
