package main

import (
	"context"
	"fmt"
	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/zinclabs/otel-example")

var gormDB *gorm.DB

func main() {
	// tp := tel.InitTracerGRPC()
	tp := InitTracerHTTP()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			fmt.Println("Error shutting down tracer provider: ", err)
		}
	}()

	// sql
	db, err := otelsql.Open("postgres", "host=127.0.0.1 port=5432 user=root password=root dbname=eye_truth_test sslmode=disable", otelsql.WithAttributes(
		semconv.DBSystemProgress,
	))
	if err != nil {
		panic(err)
	}
	gormDB, err = gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	// sql end

	router := gin.Default()

	router.Use(otelgin.Middleware(""))

	router.GET("/", GetUser)

	router.Run("127.0.0.1:8080")

}

func GetUser(c *gin.Context) {
	ctx := c.Request.Context()

	// sleep for 1 second to simulate a slow request
	time.Sleep(1 * time.Second)

	err := gormDB.WithContext(ctx).Raw(`SELECT * FROM orders`).Error
	if err != nil {
		panic(err)
	}

	childCtx, span := tracer.Start(ctx, "GetUser")
	defer span.End()

	details := GetUserDetails(childCtx)
	c.String(http.StatusOK, details)
}

func GetUserDetails(ctx context.Context) string {
	_, span := tracer.Start(ctx, "GetUserDetails")
	defer span.End()
	// sleep for 500 ms to simulate a slow request
	time.Sleep(500 * time.Millisecond)

	// log a message to stdout with the traceID and spanID
	log.Info().Str("traceID", span.SpanContext().TraceID().String()).Str("spanID", span.SpanContext().SpanID().String()).Msg("Log message for user details")

	span.AddEvent("GetUserDetails called")

	childCtx, spanHobby := tracer.Start(ctx, "GetHobbiesCall")
	defer spanHobby.End()

	hobbies, err := GetHobbies(childCtx)
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.String("hobbies", hobbies))
	}

	return "Hello User Details from Go microservice"
}

func GetHobbies(ctx context.Context) (string, error) {
	_, span := tracer.Start(ctx, "GetHobbies")

	defer func() {
		// We recover from any panics here and add the the stack trace to the span to allow for debugging
		r := recover()

		if r != nil {
			fmt.Println("recovered: ", r)

			// get error from panic
			e1 := r.(error)
			stackTrace := debug.Stack()
			runtime.Stack(stackTrace, true)

			// add attributes to the span event
			attributes := []attribute.KeyValue{
				attribute.String("user", "useremail@userdomain.com"),
				attribute.String("exception.escaped", "false"),
				attribute.String("exception.stacktrace", string(stackTrace)),
			}
			options := trace.WithAttributes(attributes...)

			// add error to span event
			span.RecordError(e1, options)

			// set event status to error
			span.SetStatus(codes.Error, e1.Error())
		}

		span.End()
	}()

	// sleep for 500 ms to simulate a slow request
	time.Sleep(500 * time.Millisecond)

	span.AddEvent("GetHobbies called")

	// We will cause a divide by zero error here
	a := 0
	b := 3
	c := b / a

	return strconv.Itoa(c), nil

}

func InitTracerHTTP() *sdktrace.TracerProvider {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	OTEL_OTLP_HTTP_ENDPOINT := os.Getenv("OTEL_OTLP_HTTP_ENDPOINT")

	if OTEL_OTLP_HTTP_ENDPOINT == "" {
		OTEL_OTLP_HTTP_ENDPOINT = "localhost:5080" //without trailing slash
	}

	otlptracehttp.NewClient()

	otlpHTTPExporter, err := otlptracehttp.New(context.TODO(),
		//otlptracehttp.WithInsecure(), // use http & not https
		otlptracehttp.WithEndpoint("api.openobserve.ai"),
		otlptracehttp.WithURLPath("/api/aa_organization_20677_e6yeLroavneMCVN/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic YWRhcGF3YW5nQGdtYWlsLmNvbTo5YmE1Mk8wSzc4WjE0WXJFNjNRTg==",
		}),
	)

	if err != nil {
		fmt.Println("Error creating HTTP OTLP exporter: ", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("otel1-gin-gonic"),
		semconv.ServiceVersionKey.String("0.0.1"),
		attribute.String("environment", "test"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(otlpHTTPExporter),
	)
	otel.SetTracerProvider(tp)

	return tp
}
