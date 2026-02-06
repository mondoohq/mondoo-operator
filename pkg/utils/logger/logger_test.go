// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

type testEncoder struct {
	result string
}

func (t *testEncoder) AppendString(s string) {
	t.result = s
}

// Implement remaining PrimitiveArrayEncoder methods as no-ops
func (t *testEncoder) AppendBool(bool)             {}
func (t *testEncoder) AppendByteString([]byte)     {}
func (t *testEncoder) AppendComplex128(complex128) {}
func (t *testEncoder) AppendComplex64(complex64)   {}
func (t *testEncoder) AppendFloat64(float64)       {}
func (t *testEncoder) AppendFloat32(float32)       {}
func (t *testEncoder) AppendInt(int)               {}
func (t *testEncoder) AppendInt64(int64)           {}
func (t *testEncoder) AppendInt32(int32)           {}
func (t *testEncoder) AppendInt16(int16)           {}
func (t *testEncoder) AppendInt8(int8)             {}
func (t *testEncoder) AppendUint(uint)             {}
func (t *testEncoder) AppendUint64(uint64)         {}
func (t *testEncoder) AppendUint32(uint32)         {}
func (t *testEncoder) AppendUint16(uint16)         {}
func (t *testEncoder) AppendUint8(uint8)           {}
func (t *testEncoder) AppendUintptr(uintptr)       {}

func TestSeverityLevelEncoder(t *testing.T) {
	tests := []struct {
		level    zapcore.Level
		expected string
	}{
		{zapcore.DebugLevel, "DEBUG"},
		{zapcore.InfoLevel, "INFO"},
		{zapcore.WarnLevel, "WARNING"}, // Cloud log explorers expect WARNING, not WARN
		{zapcore.ErrorLevel, "ERROR"},
		{zapcore.DPanicLevel, "CRITICAL"},
		{zapcore.PanicLevel, "CRITICAL"},
		{zapcore.FatalLevel, "CRITICAL"},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			enc := &testEncoder{}
			severityLevelEncoder(tt.level, enc)
			assert.Equal(t, tt.expected, enc.result)
		})
	}
}

func TestNewLogger(t *testing.T) {
	// Ensure NewLogger creates a valid logger without panicking
	logger := NewLogger()
	assert.NotNil(t, logger)

	// Test that the logger can be used
	logger.Info("test message", "key", "value")
}
