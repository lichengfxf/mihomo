package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRawConfigHasDefaultLogFile(t *testing.T) {
	cfg := DefaultRawConfig()

	assert.Equal(t, "/var/log/sdc-mihomo/mihomo.log", cfg.LogFile)
	assert.Equal(t, "text", cfg.LogFileFormat)
	assert.True(t, cfg.LogFileAppend)
	assert.Equal(t, 20, cfg.LogFileMaxSize)
	assert.Equal(t, 5, cfg.LogFileMaxBackups)
	assert.Equal(t, 7, cfg.LogFileMaxAge)
	assert.True(t, cfg.LogFileCompress)
}

func TestParseGeneralRejectsInvalidLogFileFormat(t *testing.T) {
	cfg := DefaultRawConfig()
	cfg.LogFileFormat = "json"

	_, err := parseGeneral(cfg)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsupported log-file-format")
}

func TestParseGeneralRejectsInvalidLogRotateValues(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*RawConfig)
		wantError string
	}{
		{
			name: "max-size",
			mutate: func(cfg *RawConfig) {
				cfg.LogFileMaxSize = 0
			},
			wantError: "log-file-max-size",
		},
		{
			name: "max-backups",
			mutate: func(cfg *RawConfig) {
				cfg.LogFileMaxBackups = -1
			},
			wantError: "log-file-max-backups",
		},
		{
			name: "max-age",
			mutate: func(cfg *RawConfig) {
				cfg.LogFileMaxAge = -1
			},
			wantError: "log-file-max-age",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultRawConfig()
			tt.mutate(cfg)

			_, err := parseGeneral(cfg)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantError)
		})
	}
}
