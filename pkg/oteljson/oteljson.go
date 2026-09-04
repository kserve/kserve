/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package oteljson

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
	otlplog "go.opentelemetry.io/proto/otlp/logs/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Format selects the log output format.
type Format string

const (
	// FormatZap preserves the existing controller-runtime zap output.
	FormatZap Format = "zap"
	// FormatOTelJSON emits logs using the OpenTelemetry Logs Data Model.
	FormatOTelJSON Format = "otel-json"
)

// BindFlags adds the log format flag to fs.
func BindFlags(fs *flag.FlagSet, format *Format) {
	*format = FormatZap
	fs.Func("log-format", "Log output format (one of 'zap' or 'otel-json').", func(value string) error {
		parsed := Format(value)
		if parsed != FormatZap && parsed != FormatOTelJSON {
			return fmt.Errorf("unsupported log format %q", value)
		}
		*format = parsed
		return nil
	})
}

// Apply configures controller-runtime zap options for OTel JSON stdout logs.
func Apply(opts *crzap.Options, serviceName string, destWriter io.Writer) {
	if opts == nil {
		return
	}
	if destWriter != nil {
		opts.DestWriter = destWriter
	}
	opts.EncoderConfigOptions = append(opts.EncoderConfigOptions, func(ec *zapcore.EncoderConfig) {
		ConfigureEncoder(ec)
	})
	opts.ZapOpts = append(opts.ZapOpts,
		zap.WrapCore(WrapCore),
		zap.Fields(zap.String("service.name", ServiceName(serviceName))),
	)
}

// ConfigureEncoder sets OTel Logs Data Model JSON field names.
func ConfigureEncoder(ec *zapcore.EncoderConfig) {
	ec.TimeKey = "timestamp"
	ec.LevelKey = "severity_text"
	ec.NameKey = "logger"
	ec.CallerKey = "caller"
	ec.MessageKey = "body"
	ec.StacktraceKey = "stacktrace"
	ec.EncodeLevel = EncodeSeverityText
	ec.EncodeTime = zapcore.RFC3339NanoTimeEncoder
}

// EncodeSeverityText maps zap levels to OTel severity_text values.
func EncodeSeverityText(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(SeverityText(l))
}

// SeverityText returns the OTel severity_text for a zap level.
func SeverityText(l zapcore.Level) string {
	switch {
	case l >= zapcore.DPanicLevel:
		return "FATAL"
	case l >= zapcore.ErrorLevel:
		return "ERROR"
	case l >= zapcore.WarnLevel:
		return "WARN"
	case l >= zapcore.InfoLevel:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// SeverityNumber returns the OTel severity_number for a zap level.
func SeverityNumber(l zapcore.Level) int {
	switch {
	case l >= zapcore.DPanicLevel:
		return int(otlplog.SeverityNumber_SEVERITY_NUMBER_FATAL)
	case l >= zapcore.ErrorLevel:
		return int(otlplog.SeverityNumber_SEVERITY_NUMBER_ERROR)
	case l >= zapcore.WarnLevel:
		return int(otlplog.SeverityNumber_SEVERITY_NUMBER_WARN)
	case l >= zapcore.InfoLevel:
		return int(otlplog.SeverityNumber_SEVERITY_NUMBER_INFO)
	default:
		return int(otlplog.SeverityNumber_SEVERITY_NUMBER_DEBUG)
	}
}

// ServiceName returns OTEL_SERVICE_NAME or fallback.
func ServiceName(fallback string) string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return fallback
}

// WrapCore adds severity_number to every log record.
func WrapCore(c zapcore.Core) zapcore.Core {
	return &otelCore{Core: c}
}

type otelCore struct {
	zapcore.Core
}

func (c *otelCore) With(fields []zapcore.Field) zapcore.Core {
	return &otelCore{Core: c.Core.With(fields)}
}

func (c *otelCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *otelCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	fields = append(fields, zap.Int("severity_number", SeverityNumber(ent.Level)))
	return c.Core.Write(ent, fields)
}

// FromContext returns the logr logger from ctx with trace_id/span_id when a span is active.
func FromContext(ctx context.Context) logr.Logger {
	logger := log.FromContext(ctx)
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.WithValues("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}

// WithTrace returns logger with trace_id/span_id when a span is active on ctx.
func WithTrace(ctx context.Context, logger logr.Logger) logr.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.WithValues("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}
