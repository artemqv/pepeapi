package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

func TraceInit(
	ctx context.Context,
	ServiceName string,
) (*sdktrace.TracerProvider, error) {

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(ServiceName),
	)

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"create trace exporter: %w",
			err,
		)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	)

	otel.SetTracerProvider(provider)

	return provider, nil
}
