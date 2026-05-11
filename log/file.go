package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLogFilePath      = "/var/log/sdc-mihomo/mihomo.log"
	defaultLogFileFormat    = "text"
	defaultLogFileMaxSize   = 20
	defaultLogFileBackups   = 5
	defaultLogFileMaxAge    = 7
	internalLogErrorCoolOff = 30 * time.Second
)

type FileConfig struct {
	Path       string
	Format     string
	Append     bool
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

type fileOutput interface {
	Write(Event) error
	Close() error
}

type rotatingFileOutput struct {
	mu     sync.Mutex
	writer io.WriteCloser
}

type limitedInternalErrorReporter struct {
	mu         sync.Mutex
	lastError  string
	lastAt     time.Time
	suppressed int
}

var (
	fileOutputMu      sync.RWMutex
	currentFileOutput fileOutput
	currentFileConfig = DefaultFileConfig()
	errorReporter     limitedInternalErrorReporter
)

func DefaultFileConfig() FileConfig {
	return FileConfig{
		Path:       defaultLogFilePath,
		Format:     defaultLogFileFormat,
		Append:     true,
		MaxSize:    defaultLogFileMaxSize,
		MaxBackups: defaultLogFileBackups,
		MaxAge:     defaultLogFileMaxAge,
		Compress:   true,
	}
}

func ApplyFileConfig(cfg FileConfig) {
	output, err := newFileOutput(cfg)
	if err != nil {
		errorReporter.Report("configure file log", err)
		return
	}

	previous := swapFileOutput(output, cfg)
	if previous != nil {
		if err := previous.Close(); err != nil {
			errorReporter.Report("close old file log", err)
		}
	}
	errorReporter.Reset()
}

func writeFile(event Event) {
	fileOutputMu.RLock()
	output := currentFileOutput
	fileOutputMu.RUnlock()
	if output == nil {
		return
	}

	if err := output.Write(event); err != nil {
		errorReporter.Report("write file log", err)
	}
}

func CloseFileOutput() {
	previous := swapFileOutput(nil, DefaultFileConfig())
	if previous == nil {
		return
	}

	if err := previous.Close(); err != nil {
		errorReporter.Report("close file log", err)
	}
	errorReporter.Reset()
}

func FileOutputConfig() *FileConfig {
	fileOutputMu.RLock()
	defer fileOutputMu.RUnlock()
	cfg := currentFileConfig
	return &cfg
}

func ValidateFileConfig(cfg FileConfig) error {
	return validateFileConfig(cfg)
}

func newFileOutput(cfg FileConfig) (fileOutput, error) {
	if err := validateFileConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.Path == "" {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, err
	}

	writer, err := newRotatingWriter(cfg)
	if err != nil {
		return nil, err
	}

	return &rotatingFileOutput{writer: writer}, nil
}

func validateFileConfig(cfg FileConfig) error {
	if cfg.Path == "" {
		return nil
	}
	if cfg.Format == "" {
		cfg.Format = defaultLogFileFormat
	}
	if cfg.Format != defaultLogFileFormat {
		return fmt.Errorf("unsupported log-file-format: %s", cfg.Format)
	}
	if cfg.MaxSize <= 0 {
		return fmt.Errorf("log-file-max-size must be greater than 0")
	}
	if cfg.MaxBackups < 0 {
		return fmt.Errorf("log-file-max-backups must be greater than or equal to 0")
	}
	if cfg.MaxAge < 0 {
		return fmt.Errorf("log-file-max-age must be greater than or equal to 0")
	}
	return nil
}

func newRotatingWriter(cfg FileConfig) (io.WriteCloser, error) {
	if cfg.Append {
		return &lumberjack.Logger{
			Filename:   cfg.Path,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
			LocalTime:  true,
		}, nil
	}

	file, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	return &lumberjack.Logger{
		Filename:   cfg.Path,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}, nil
}

func (o *rotatingFileOutput) Write(event Event) error {
	line := fmt.Sprintf("%s [%s] %s\n",
		time.Now().Format(time.RFC3339Nano),
		event.LogLevel.String(),
		event.Payload,
	)

	o.mu.Lock()
	defer o.mu.Unlock()
	_, err := io.WriteString(o.writer, line)
	return err
}

func (o *rotatingFileOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writer == nil {
		return nil
	}
	err := o.writer.Close()
	o.writer = nil
	return err
}

func (o *rotatingFileOutput) Config() FileConfig {
	return currentFileConfig
}

func swapFileOutput(next fileOutput, cfg FileConfig) fileOutput {
	fileOutputMu.Lock()
	defer fileOutputMu.Unlock()
	previous := currentFileOutput
	currentFileOutput = next
	currentFileConfig = cfg
	return previous
}

func (r *limitedInternalErrorReporter) Report(action string, err error) {
	if err == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	current := fmt.Sprintf("%s: %v", action, err)
	if r.lastError == current && now.Sub(r.lastAt) < internalLogErrorCoolOff {
		r.suppressed++
		return
	}

	if r.suppressed > 0 && r.lastError == current {
		_, _ = fmt.Fprintf(os.Stderr, "[mihomo] file log error persists, suppressed=%d\n", r.suppressed)
		r.suppressed = 0
	}

	if r.lastError != "" && r.lastError != current {
		_, _ = fmt.Fprintf(os.Stderr, "[mihomo] file log recovered: %s\n", r.lastError)
	}

	_, _ = fmt.Fprintf(os.Stderr, "[mihomo] %s\n", current)
	r.lastError = current
	r.lastAt = now
}

func (r *limitedInternalErrorReporter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastError != "" {
		_, _ = fmt.Fprintf(os.Stderr, "[mihomo] file log recovered: %s\n", r.lastError)
	}
	r.lastError = ""
	r.lastAt = time.Time{}
	r.suppressed = 0
}
