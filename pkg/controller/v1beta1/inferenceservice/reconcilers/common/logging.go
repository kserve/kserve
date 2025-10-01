package common

import (
	"context"

	"github.com/go-logr/logr"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// LoggerWithFallback returns a logger sourced from the provided context if available,
// otherwise it falls back to the supplied logger. Callers should invoke this helper at
// the start of every reconciliation entrypoint so each function refreshes its context
// with the trace-aware logger that belongs to the current request. Doing this once per
// package would freeze the logger to whichever request happened to run first, losing
// per-request fields such as trace IDs for all subsequent reconciliations. The returned
// context will be updated to contain the logger that was selected so downstream calls
// can retrieve it.
func LoggerWithFallback(ctx context.Context, fallback logr.Logger) (context.Context, logr.Logger, bool) {
	log := logr.FromContextOrDiscard(ctx)
	fromContext := false
	if log.GetSink() != nil {
		fromContext = true
	} else {
		log = fallback
	}
	ctx = logr.NewContext(ctx, log)
	return ctx, log, fromContext
}

// LoggerForContext returns a logger sourced from the provided context if available and enriches it with the
// supplied name and trace identifier. The updated logger is stored back in the context before being returned.
func LoggerForContext(ctx context.Context, fallback logr.Logger, name string) (context.Context, logr.Logger) {
	ctx, log, fromContext := LoggerWithFallback(ctx, fallback)
	if name != "" && fromContext {
		log = log.WithName(name)
	}
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() && spanCtx.HasTraceID() {
		log = log.WithValues("trace_id", spanCtx.TraceID().String())
	}
	ctx = logr.NewContext(ctx, log)
	return ctx, log
}
