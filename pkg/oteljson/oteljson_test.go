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

package oteljson_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	otlplog "go.opentelemetry.io/proto/otlp/logs/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kserve/kserve/pkg/oteljson"
)

func TestSeverityMapping(t *testing.T) {
	assert.Equal(t, "INFO", oteljson.SeverityText(zapcore.InfoLevel))
	assert.Equal(t, int(otlplog.SeverityNumber_SEVERITY_NUMBER_INFO), oteljson.SeverityNumber(zapcore.InfoLevel))
	assert.Equal(t, "WARN", oteljson.SeverityText(zapcore.WarnLevel))
	assert.Equal(t, int(otlplog.SeverityNumber_SEVERITY_NUMBER_WARN), oteljson.SeverityNumber(zapcore.WarnLevel))
	assert.Equal(t, "ERROR", oteljson.SeverityText(zapcore.ErrorLevel))
	assert.Equal(t, int(otlplog.SeverityNumber_SEVERITY_NUMBER_ERROR), oteljson.SeverityNumber(zapcore.ErrorLevel))
	assert.Equal(t, "DEBUG", oteljson.SeverityText(zapcore.DebugLevel))
	assert.Equal(t, int(otlplog.SeverityNumber_SEVERITY_NUMBER_DEBUG), oteljson.SeverityNumber(zapcore.DebugLevel))
}

func TestBindFlagsDefaultsToZapAndAcceptsOTelJSON(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	format := oteljson.FormatOTelJSON
	oteljson.BindFlags(fs, &format)

	require.NoError(t, fs.Parse([]string{"-log-format=otel-json"}))
	assert.Equal(t, oteljson.FormatOTelJSON, format)
}

func TestBindFlagsRejectsUnknownFormat(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var format oteljson.Format
	oteljson.BindFlags(fs, &format)

	assert.Error(t, fs.Parse([]string{"-log-format=unknown"}))
	assert.Equal(t, oteljson.FormatZap, format)
}

func TestApplyUsesExplicitWriter(t *testing.T) {
	var buf bytes.Buffer
	opts := &crzap.Options{}

	oteljson.Apply(opts, "test-service", &buf)

	assert.Same(t, &buf, opts.DestWriter)
}

func TestJSONRecordUsesOTelFields(t *testing.T) {
	var buf bytes.Buffer
	encCfg := zap.NewProductionEncoderConfig()
	oteljson.ConfigureEncoder(&encCfg)
	core := oteljson.WrapCore(zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(&buf), zapcore.InfoLevel))
	zl := zap.New(core)
	zl.Info("controller started")
	require.NoError(t, zl.Sync())

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	assert.Equal(t, "controller started", rec["body"])
	assert.Equal(t, "INFO", rec["severity_text"])
	assert.Equal(t, float64(9), rec["severity_number"])
	assert.NotEmpty(t, rec["timestamp"])
}

func TestFromContextInjectsTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	encCfg := zap.NewProductionEncoderConfig()
	oteljson.ConfigureEncoder(&encCfg)
	zl := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(&buf), zapcore.InfoLevel))

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	ctx, span := tp.Tracer("test").Start(t.Context(), "reconcile")
	defer span.End()
	ctx = log.IntoContext(ctx, zapr.NewLogger(zl))

	oteljson.FromContext(ctx).Info("reconciling")
	require.NoError(t, zl.Sync())

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	sc := span.SpanContext()
	assert.Equal(t, sc.TraceID().String(), rec["trace_id"])
	assert.Equal(t, sc.SpanID().String(), rec["span_id"])
}
