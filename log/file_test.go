package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyFileConfigWritesLogFile(t *testing.T) {
	t.Cleanup(CloseFileOutput)

	dir := t.TempDir()
	path := filepath.Join(dir, "mihomo.log")

	ApplyFileConfig(FileConfig{
		Path:       path,
		Format:     "text",
		Append:     true,
		MaxSize:    1,
		MaxBackups: 1,
		MaxAge:     1,
		Compress:   false,
	})

	writeFile(Event{LogLevel: INFO, Payload: "hello file"})
	CloseFileOutput()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[info] hello file")
}

func TestApplyFileConfigDisableWithEmptyPath(t *testing.T) {
	t.Cleanup(CloseFileOutput)

	dir := t.TempDir()
	path := filepath.Join(dir, "mihomo.log")

	ApplyFileConfig(FileConfig{
		Path:       path,
		Format:     "text",
		Append:     true,
		MaxSize:    1,
		MaxBackups: 1,
		MaxAge:     1,
		Compress:   false,
	})
	writeFile(Event{LogLevel: INFO, Payload: "before disable"})

	ApplyFileConfig(FileConfig{
		Path:       "",
		Format:     "text",
		Append:     true,
		MaxSize:    1,
		MaxBackups: 1,
		MaxAge:     1,
		Compress:   false,
	})
	writeFile(Event{LogLevel: INFO, Payload: "after disable"})
	CloseFileOutput()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "before disable")
	assert.NotContains(t, content, "after disable")
}

func TestApplyFileConfigReloadSwitchesPath(t *testing.T) {
	t.Cleanup(CloseFileOutput)

	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.log")
	pathB := filepath.Join(dir, "b.log")

	cfg := FileConfig{
		Format:     "text",
		Append:     true,
		MaxSize:    1,
		MaxBackups: 1,
		MaxAge:     1,
		Compress:   false,
	}

	cfg.Path = pathA
	ApplyFileConfig(cfg)
	writeFile(Event{LogLevel: INFO, Payload: "path a"})

	cfg.Path = pathB
	ApplyFileConfig(cfg)
	writeFile(Event{LogLevel: INFO, Payload: "path b"})
	CloseFileOutput()

	dataA, err := os.ReadFile(pathA)
	require.NoError(t, err)
	assert.Contains(t, string(dataA), "path a")
	assert.NotContains(t, string(dataA), "path b")

	dataB, err := os.ReadFile(pathB)
	require.NoError(t, err)
	assert.NotContains(t, string(dataB), "path a")
	assert.Contains(t, string(dataB), "path b")
}

func TestApplyFileConfigRotateCreatesBackup(t *testing.T) {
	t.Cleanup(CloseFileOutput)

	dir := t.TempDir()
	path := filepath.Join(dir, "mihomo.log")

	ApplyFileConfig(FileConfig{
		Path:       path,
		Format:     "text",
		Append:     true,
		MaxSize:    1,
		MaxBackups: 2,
		MaxAge:     1,
		Compress:   false,
	})

	payload := strings.Repeat("x", 300*1024)
	for i := 0; i < 8; i++ {
		writeFile(Event{LogLevel: INFO, Payload: payload})
	}
	CloseFileOutput()

	matches, err := filepath.Glob(filepath.Join(dir, "mihomo-*.log"))
	require.NoError(t, err)
	assert.NotEmpty(t, matches)
}
