// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package logger

import (
	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// severityLevelEncoder encodes log levels to cloud-compatible severity names.
// Uses "WARNING" instead of zap's "WARN", and maps DPANIC/PANIC/FATAL to CRITICAL.
func severityLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
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
		// Use production mode for JSON output (enables structured logging in cloud environments).
		// Can be overridden with --zap-devel flag for local development.
		Development:     false,
		StacktraceLevel: zapcore.DPanicLevel,
		TimeEncoder:     zapcore.RFC3339TimeEncoder,
		// Configure encoder for cloud logging compatibility:
		// - Use "severity" field name (recognized by GKE, AKS, EKS log explorers)
		// - Use standard severity names (WARNING instead of WARN, CRITICAL for fatal errors)
		EncoderConfigOptions: []zap.EncoderConfigOption{
			func(ec *zapcore.EncoderConfig) {
				ec.LevelKey = "severity"
				ec.EncodeLevel = severityLevelEncoder
			},
		},
	}
	return zap.New(zap.UseFlagOptions(&opts))
}
