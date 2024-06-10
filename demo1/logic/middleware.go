package logic

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"os"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	otlptracegrpc.NewClient()
	otlpGRPCExporter, err := otlptracegrpc.New(context.TODO(),
		otlptracegrpc.WithInsecure(), // use http & not https
		otlptracegrpc.WithEndpoint("api.openobserve.ai:5081"),
		//otlptracegrpc.w ("/api/prabhat-org2/traces"),
		otlptracegrpc.WithHeaders(map[string]string{
			"Authorization": "Basic YWRhcGF3YW5nQGdtYWlsLmNvbTo5YmE1Mk8wSzc4WjE0WXJFNjNRTg==",
			"zinc-org-id":   "aa_organization_20677_e6yeLroavneMCVN",
			"organization":  "aa_organization_20677_e6yeLroavneMCVN",
			"stream-name":   "default",
		}),
	)
	if err != nil {
		fmt.Println("Error creating HTTP OTLP exporter: ", err)
		os.Exit(0)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String("otel1-gin-gonic"),
	)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(otlpGRPCExporter, res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(opt *otlptrace.Exporter, res *resource.Resource) (*trace.TracerProvider, error) {
	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(res),
		trace.WithBatcher(opt),
	)
	return traceProvider, nil
}
