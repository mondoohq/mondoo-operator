package logger

import (
	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func NewLogger() logr.Logger {
	opts := zap.Options{
		Development:     true,
		StacktraceLevel: zapcore.DPanicLevel,
		TimeEncoder:     zapcore.RFC3339TimeEncoder,
	}
	return zap.New(zap.UseFlagOptions(&opts))
}
