package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	eventlib "github.com/sammyjroberts/eventlibgo"
	"go.uber.org/zap"
)

var (
	eventsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "eventlibgo_http_events_received_total",
		Help: "Total number of events received via HTTP",
	}, []string{"type", "source"})

	eventsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "eventlibgo_http_events_processed_total",
		Help: "Total number of events processed",
	}, []string{"type", "source"})

	queueSizeGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "eventlibgo_http_queue_size",
		Help: "Current event queue size",
	})

	processingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "eventlibgo_http_processing_duration_seconds",
		Help:    "Event processing duration",
		Buckets: prometheus.DefBuckets,
	})
)

// Server wraps the event processor with HTTP handlers
type Server struct {
	processor *eventlib.EventProcessor
	logger    *zap.Logger

	// Event broadcasting
	eventBroadcast chan eventlib.Event
}

// NewServer creates a new HTTP server wrapping the event processor
func NewServer(name string, queueSize int, logger *zap.Logger) (*Server, error) {
	s := &Server{
		logger: logger,
	}

	// Configure processor
	config := &eventlib.Config{
		Name:          name,
		MaxQueueSize:  queueSize,
		EnableLogging: true,
		Logger:        logger,
	}

	handlers := &eventlib.Handlers{
		OnEvent:       s.onEvent,
		OnFilter:      s.onFilter,
		OnStateChange: s.onStateChange,
	}

	processor, err := eventlib.New(config, handlers)
	if err != nil {
		return nil, fmt.Errorf("failed to create processor: %w", err)
	}

	s.processor = processor

	// Start processor
	if err := processor.Start(); err != nil {
		processor.Close()
		return nil, fmt.Errorf("failed to start processor: %w", err)
	}

	// Start background tasks
	go s.updateMetrics()

	return s, nil
}

// Close shuts down the server
func (s *Server) Close() error {
	close(s.eventBroadcast)
	return s.processor.Close()
}

// Event handlers
func (s *Server) onEvent(event eventlib.Event) {
	eventsProcessed.WithLabelValues(
		event.Type.String(),
		event.Source,
	).Inc()

	s.logger.Info("Event processed",
		zap.String("type", event.Type.String()),
		zap.String("source", event.Source),
		zap.Int("data_len", len(event.Data)))
}

func (s *Server) onFilter(event eventlib.Event) bool {
	// Example: filter out events from "blocked" sources
	if event.Source == "blocked" {
		s.logger.Debug("Event filtered",
			zap.String("source", event.Source))
		return false
	}
	return true
}

func (s *Server) onStateChange(oldState, newState string) {
	s.logger.Info("Processor state changed",
		zap.String("from", oldState),
		zap.String("to", newState))
}

// HTTP handlers
func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	event := eventlib.Event{
		Type:   eventlib.EventType(req.Type),
		Source: req.Source,
		Data:   req.Data,
	}

	if err := s.processor.Push(event); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "Failed to queue event")
		return
	}

	eventsReceived.WithLabelValues(
		event.Type.String(),
		event.Source,
	).Inc()

	s.writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
	})
}

func (s *Server) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	var req BatchEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	queued := 0
	failed := 0

	for _, e := range req.Events {
		event := eventlib.Event{
			Type:   eventlib.EventType(e.Type),
			Source: e.Source,
			Data:   e.Data,
		}

		if err := s.processor.Push(event); err != nil {
			failed++
			s.logger.Warn("Failed to queue event in batch",
				zap.Error(err),
				zap.Int("index", queued+failed))
		} else {
			queued++
			eventsReceived.WithLabelValues(
				event.Type.String(),
				event.Source,
			).Inc()
		}
	}

	s.writeJSON(w, http.StatusAccepted, map[string]int{
		"queued": queued,
		"failed": failed,
	})
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.processor.Process()
	processingDuration.Observe(time.Since(start).Seconds())

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "processed",
	})
}

func (s *Server) handleProcessAll(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	before := s.processor.EventsProcessed()

	s.processor.ProcessAll()

	after := s.processor.EventsProcessed()
	processingDuration.Observe(time.Since(start).Seconds())

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "processed",
		"processed": after - before,
		"duration":  time.Since(start).String(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := StatusResponse{
		State:           s.processor.State(),
		QueueSize:       s.processor.QueueSize(),
		EventsProcessed: s.processor.EventsProcessed(),
		Timestamp:       time.Now(),
	}

	s.writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status: "healthy",
		Checks: map[string]bool{
			"processor": s.processor.State() == "RUNNING",
			"queue":     s.processor.QueueSize() < 9000, // 90% threshold
		},
	}

	// Determine overall health
	for _, check := range health.Checks {
		if !check {
			health.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
			break
		}
	}

	s.writeJSON(w, http.StatusOK, health)
}

// Helper methods
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

func (s *Server) updateMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		queueSizeGauge.Set(float64(s.processor.QueueSize()))
	}
}
