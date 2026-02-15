package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracer initializes the OTel SDK
func InitTracer(serviceName string) (func(context.Context) error, error) {
	ctx := context.Background()

	// Configure the OTLP HTTP exporter to send traces to Jaeger (localhost:4318)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithInsecure(), // Use HTTP, not HTTPS for local Jaeger
		otlptracehttp.WithEndpoint("localhost:4319"),
	)
	if err != nil {
		return nil, err
	}

	// Create a resource to describe the app (Service Name)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create the TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// Set the global TracerProvider
	otel.SetTracerProvider(tp)

	// Set the global Propagator (W3C is standard)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp.Shutdown, nil
}
