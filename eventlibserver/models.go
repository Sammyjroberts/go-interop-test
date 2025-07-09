package main

import "time"

// EventRequest represents a single event POST request
type EventRequest struct {
	Type   int    `json:"type"`
	Source string `json:"source"`
	Data   []byte `json:"data,omitempty"`
}

// BatchEventRequest represents multiple events
type BatchEventRequest struct {
	Events []EventRequest `json:"events"`
}

// StatusResponse represents the processor status
type StatusResponse struct {
	State           string    `json:"state"`
	QueueSize       int       `json:"queue_size"`
	EventsProcessed int       `json:"events_processed"`
	Timestamp       time.Time `json:"timestamp"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status string          `json:"status"`
	Checks map[string]bool `json:"checks"`
}

// EventMessage for WebSocket streaming
type EventMessage struct {
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Data      []byte    `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
