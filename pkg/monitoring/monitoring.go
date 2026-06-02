package monitoring

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const meterName = "mongodb-migrator"

var (
	commonMeter metric.Meter
)

// Init initializes the global OpenTelemetry MeterProvider and TracerProvider,
// and sets up the common meter using standard OTLP gRPC exporters.
func Init(ctx context.Context) (func(), error) {
	// 1. Define the application's identity
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"", // Use empty SchemaURL to avoid conflicts during merging with Default resource
			attribute.String("service.name", meterName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 2. Setup OTLP Trace Exporter (Defaults to localhost:4317)
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// 3. Setup OTLP Metric Exporter (Defaults to localhost:4317)
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// Initialize the common meter
	commonMeter = mp.Meter(meterName)

	shutdown := func() {
		log.Println("OpenTelemetry monitoring shutting down and flushing telemetry...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := tp.Shutdown(shutdownCtx); err != nil {
			otel.Handle(err)
			log.Printf("Failed to shut down OpenTelemetry TracerProvider: %v", err)
		}

		if err := mp.Shutdown(shutdownCtx); err != nil {
			otel.Handle(err)
			log.Printf("Failed to shut down OpenTelemetry MeterProvider: %v", err)
		} else {
			log.Println("OpenTelemetry monitoring successfully shut down and telemetry flushed")
		}
	}

	return shutdown, nil
}

// Meter returns the common meter instance.
func Meter() metric.Meter {
	if commonMeter == nil {
		// Fallback to global meter if not initialized yet
		return otel.Meter(meterName)
	}
	return commonMeter
}

// NewDocumentsReadCounter initializes and returns the documents_read_total counter.
func NewDocumentsReadCounter() (metric.Int64Counter, error) {
	return Meter().Int64Counter("documents_read_total",
		metric.WithDescription("Total number of documents read during backfill"),
		metric.WithUnit("{document}"),
	)
}

