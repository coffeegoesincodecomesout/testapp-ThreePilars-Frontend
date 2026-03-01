package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/trace"
)

var pingCounter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "ping_request_count",
		Help: "No of request handled by Ping handler",
	},
)

func main() {
	ctx := context.Background()
	exp, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		panic(err)
	}

	tracerProvider := trace.NewTracerProvider(trace.WithBatcher(exp))
	defer func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			panic(err)
		}
	}()
	otel.SetTracerProvider(tracerProvider)

	prometheus.MustRegister(pingCounter)

	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		backendURL = "http://threepilar-backend-service:8091"
	}

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		tracer := otel.Tracer("http-server")
		_, span := tracer.Start(r.Context(), "handleRequest")
		defer span.End()

		time.Sleep(1 * time.Second)

		resp, err := http.Get(backendURL + "/response")
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, "backend error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		span.SetStatus(codes.Ok, "Status 200")
		pingCounter.Inc()
		fmt.Fprintf(w, string(body))

		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
		logger.Info("ping...response...", "message", string(body))
	})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Server is starting...")

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8090", nil)
}
