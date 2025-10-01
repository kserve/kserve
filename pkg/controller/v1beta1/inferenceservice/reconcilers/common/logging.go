package common

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/go-logr/logr"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type traceMetadata struct {
	id       string
	fromSpan bool
	logged   bool
}

type traceMetadataKey struct{}

var traceKey = traceMetadataKey{}

// LoggerWithFallback returns a logger sourced from the provided context if available,
// otherwise it falls back to the supplied logger. Callers should invoke this helper at
// the start of every reconciliation entrypoint so each function refreshes its context
// with the trace-aware logger that belongs to the current request. Doing this once per
// package would freeze the logger to whichever request happened to run first, losing
// per-request fields such as trace IDs for all subsequent reconciliations. When the
// fallback logger is selected it is stored in the returned context, enabling downstream
// functions to retrieve it via logr.FromContext.
func LoggerWithFallback(ctx context.Context, fallback logr.Logger) (context.Context, logr.Logger, bool) {
	log := logr.FromContextOrDiscard(ctx)
	if log.GetSink() != nil {
		return ctx, log, true
	}
	log = fallback
	ctx = logr.NewContext(ctx, log)
	return ctx, log, false
}

// LoggerForContext returns a logger sourced from the provided context if available and enriches it with the
// supplied name and trace identifier. If the logger is modified, the enriched version is stored back in the
// returned context so subsequent calls observe the additional metadata.
func LoggerForContext(ctx context.Context, fallback logr.Logger, name string) (context.Context, logr.Logger) {
	ctx, log, fromContext := LoggerWithFallback(ctx, fallback)

	var md traceMetadata
	if existing, ok := ctx.Value(traceKey).(traceMetadata); ok {
		md = existing
	}

	if !md.logged {
		ctx, log = enrichLoggerWithTrace(ctx, log, fromContext, name, md)
	} else if name != "" && fromContext {
		log = log.WithName(name)
		ctx = logr.NewContext(ctx, log)
	}

	return ctx, log
}

func enrichLoggerWithTrace(ctx context.Context, log logr.Logger, fromContext bool, name string, md traceMetadata) (context.Context, logr.Logger) {
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	modified := false

	if name != "" && fromContext {
		log = log.WithName(name)
		modified = true
	}

	if spanCtx.IsValid() && spanCtx.HasTraceID() {
		md.id = spanCtx.TraceID().String()
		md.fromSpan = true
	} else if md.id == "" {
		if generated, ok := generateTraceID(); ok {
			md.id = generated
		}
	}

	if md.id != "" {
		log = log.WithValues("trace_id", md.id)
		md.logged = true
		modified = true
	}

	if modified {
		ctx = logr.NewContext(ctx, log)
	}

	ctx = context.WithValue(ctx, traceKey, md)

	return ctx, log
}

func generateTraceID() (string, bool) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false
	}
	return hex.EncodeToString(b[:]), true
}
