package trace

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var GlobalTracer *Tracer

type RequestId string

const RequestIdKey = RequestId("request-id")

type TracerOpts struct {
	TracingInsecure bool
	CollectorUrl    string
	Name            string
}

type Tracer struct {
	opts       TracerOpts
	shutdownCb func(context.Context) error
}

func New(opts TracerOpts) *Tracer {
	log.Debug().Msg("starting tracing module")
	// secureOption := otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, ""))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	secureOption := otlptracegrpc.WithInsecure()

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracegrpc.NewClient(
			secureOption,
			otlptracegrpc.WithEndpoint(opts.CollectorUrl),
		),
	)

	if err != nil {
		log.Fatal().Err(err)
	}
	resources, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", opts.Name),
			attribute.String("library.language", "go"),
		),
	)
	if err != nil {
		log.Error().Err(err).Msg("Could not set resources")
	}

	otel.SetTracerProvider(
		sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(resources),
		),
	)
	log.Debug().Msg("finished tracing module initiation")

	t := &Tracer{
		opts:       opts,
		shutdownCb: exporter.Shutdown,
	}

	GlobalTracer = t

	return t
}

func (t *Tracer) Shutdown(ctx context.Context) error {
	return t.shutdownCb(ctx)
}

func (t *Tracer) TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestId := r.Header.Get("x-request-id")
		if requestId == "" {
			requestId = uuid.NewString()
		}
		tracer := otel.GetTracerProvider()
		ctx, span := tracer.Tracer(GlobalTracer.opts.Name).Start(ctx, r.URL.Path)

		ctx = context.WithValue(ctx, RequestIdKey, requestId)
		next.ServeHTTP(w, r.WithContext(ctx))

		defer span.End()
	})
}
