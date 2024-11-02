package trace

import (
	"context"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func WithSpan(ctx context.Context, fn func(ctx context.Context, span trace.Span) (any, error)) (any, error) {
	tracer := otel.GetTracerProvider()
	if tracer == nil {
		return fn(ctx, nil)
	}
	pc, _, _, _ := runtime.Caller(1)
	fullFuncName := runtime.FuncForPC(pc).Name()
	ctx, span := tracer.Tracer(GlobalTracer.opts.Name).Start(ctx, fullFuncName)

	defer span.End()

	return fn(ctx, span)
}
