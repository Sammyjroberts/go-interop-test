package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var (
	addr          = flag.String("addr", ":8080", "HTTP server address")
	metricsAddr   = flag.String("metrics-addr", ":9090", "Metrics server address")
	queueSize     = flag.Int("queue-size", 10000, "Maximum event queue size")
	processorName = flag.String("name", "HTTPEventProcessor", "Processor name")
)

func main() {
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create server
	srv, err := NewServer(*processorName, *queueSize, logger)
	if err != nil {
		logger.Fatal("Failed to create server", zap.Error(err))
	}
	defer srv.Close()

	// Setup routes
	router := mux.NewRouter()

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()
	api.Use(srv.loggingMiddleware)
	api.Use(srv.metricsMiddleware)

	api.HandleFunc("/events", srv.handlePostEvent).Methods("POST")
	api.HandleFunc("/events/batch", srv.handleBatchEvents).Methods("POST")
	api.HandleFunc("/process", srv.handleProcess).Methods("POST")
	api.HandleFunc("/process/all", srv.handleProcessAll).Methods("POST")
	api.HandleFunc("/status", srv.handleStatus).Methods("GET")
	api.HandleFunc("/health", srv.handleHealth).Methods("GET")

	// Metrics server
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    *metricsAddr,
		Handler: metricsMux,
	}

	// Start server
	httpServer := &http.Server{
		Addr:         *addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan struct{})
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("Shutting down servers...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		httpServer.Shutdown(ctx)
		metricsServer.Shutdown(ctx)
		close(done)
	}()

	// Start metrics server
	go func() {
		logger.Info("Starting metrics server", zap.String("addr", *metricsAddr))
		if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	// Start main server
	logger.Info("Starting HTTP server", zap.String("addr", *addr))
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		logger.Fatal("HTTP server error", zap.Error(err))
	}

	<-done
	logger.Info("Server stopped")
}
