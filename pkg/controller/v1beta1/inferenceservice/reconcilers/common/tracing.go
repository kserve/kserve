package common

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const tracerName = "github.com/kserve/kserve/controller"

var defaultTracer = otel.Tracer(tracerName)

// tracingReconciler decorates a reconcile.Reconciler to start an OpenTelemetry span
// for each reconciliation and ensures the context is seeded with the trace-aware logger.
type tracingReconciler struct {
	delegate reconcile.Reconciler
	tracer   trace.Tracer
	fallback logr.Logger
	spanName string
}

// WrapWithTracing returns a Reconciler that instruments the delegate with an OTEL span.
// spanName is optional; when empty the delegate type name is used.
func WrapWithTracing(delegate reconcile.Reconciler, fallback logr.Logger, spanName string) reconcile.Reconciler {
	if spanName == "" {
		spanName = deriveSpanName(delegate)
	}

	return &tracingReconciler{
		delegate: delegate,
		tracer:   defaultTracer,
		fallback: fallback,
		spanName: spanName,
	}
}

func (t *tracingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := t.tracer.Start(ctx, t.spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("k8s.namespace", req.Namespace),
			attribute.String("k8s.name", req.Name),
			attribute.String("controller", t.spanName),
		),
	)
	start := time.Now()
	defer span.End()

	ctx, log := LoggerForContext(ctx, t.fallback, "")
	log = log.WithValues("controller", t.spanName)
	ctx = logr.NewContext(ctx, log)

	res, err := t.delegate.Reconcile(ctx, req)
	span.SetAttributes(
		attribute.Bool("controller.requeue", res.Requeue),
		attribute.Bool("controller.requeue_after_set", res.RequeueAfter > 0),
		attribute.Float64("controller.duration_ms", float64(time.Since(start).Milliseconds())),
	)

	if res.RequeueAfter > 0 {
		span.SetAttributes(attribute.String("controller.requeue_after", res.RequeueAfter.String()))
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "success")
	}

	return res, err
}

func deriveSpanName(delegate reconcile.Reconciler) string {
	type namer interface {
		Name() string
	}
	if n, ok := delegate.(namer); ok {
		if v := n.Name(); v != "" {
			return v
		}
	}
	return fmt.Sprintf("%T", delegate)
}
