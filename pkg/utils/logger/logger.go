// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package logger

import (
	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// gcpLevelEncoder encodes log levels to Google Cloud Logging severity names.
// GCP expects "WARNING" instead of zap's "WARN", and maps DPANIC/PANIC/FATAL to CRITICAL.
func gcpLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch level {
	case zapcore.WarnLevel:
		enc.AppendString("WARNING")
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		enc.AppendString("CRITICAL")
	default:
		enc.AppendString(level.CapitalString())
	}
}

func NewLogger() logr.Logger {
	opts := zap.Options{
		// Use production mode by default for JSON output (required for GKE log parsing).
		// Can be overridden with --zap-devel flag for local development.
		Development:     false,
		StacktraceLevel: zapcore.DPanicLevel,
		TimeEncoder:     zapcore.RFC3339TimeEncoder,
		// Configure encoder for GCP/GKE compatibility:
		// - Use "severity" instead of "level" for log level field
		// - Use GCP-compatible level names (WARNING instead of WARN)
		EncoderConfigOptions: []zap.EncoderConfigOption{
			func(ec *zapcore.EncoderConfig) {
				ec.LevelKey = "severity"
				ec.EncodeLevel = gcpLevelEncoder
			},
		},
	}
	return zap.New(zap.UseFlagOptions(&opts))
}
