package eventlib

/*
#cgo CFLAGS: -I${SRCDIR}/../eventlib
#cgo LDFLAGS: ${SRCDIR}/../eventlib/libeventlib.a
#include "eventlib.h"
#include <stdlib.h>

// Forward declarations for Go callbacks
extern void goHandleEvent(void* event, void* user_data);
extern void goHandleLog(void* level, void* message, void* user_data);
extern int goHandleFilter(void* event, void* user_data);
extern void goHandleStateChange(void* old_state, void* new_state, void* user_data);

// C wrapper functions that call Go
static void c_handle_event(const event_t* event, void* user_data) {
    goHandleEvent((void*)event, user_data);
}

static void c_handle_log(const char* level, const char* message, void* user_data) {
    goHandleLog((void*)level, (void*)message, user_data);
}

static bool c_handle_filter(const event_t* event, void* user_data) {
    return goHandleFilter((void*)event, user_data) != 0;
}

static void c_handle_state_change(const char* old_state, const char* new_state, void* user_data) {
    goHandleStateChange((void*)old_state, (void*)new_state, user_data);
}

// Helper to create processor with Go callbacks
static event_processor_t* create_processor_go(const char* name, size_t max_queue_size,
                                              bool enable_logging, void* user_data) {
    event_config_t config = {
        .name = name,
        .max_queue_size = max_queue_size,
        .enable_logging = enable_logging,
        .on_event = c_handle_event,
        .on_log = c_handle_log,
        .on_filter = c_handle_filter,
        .on_state_change = c_handle_state_change,
        .user_data = user_data
    };
    return event_processor_create(&config);
}
*/
import "C"
import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"go.uber.org/zap"
)

// EventProcessor wraps the C event processor
type EventProcessor struct {
	cptr     *C.event_processor_t
	config   *Config
	handlers *Handlers
	logger   *zap.Logger
	mu       sync.RWMutex
	closed   bool
}

// Config holds processor configuration
type Config struct {
	Name          string
	MaxQueueSize  int
	EnableLogging bool
	Logger        *zap.Logger
}

// Handlers contains all callback functions
type Handlers struct {
	OnEvent       EventHandler
	OnFilter      FilterHandler
	OnStateChange StateChangeHandler
}

// New creates a new event processor
func New(config *Config, handlers *Handlers) (*EventProcessor, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if handlers == nil {
		handlers = &Handlers{}
	}

	// Default logger if not provided
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	ep := &EventProcessor{
		config:   config,
		handlers: handlers,
		logger:   logger,
	}

	// Store in global map for callback access
	callbackMu.Lock()
	callbackID := nextCallbackID
	nextCallbackID++
	callbackMap[callbackID] = ep
	callbackMu.Unlock()

	// Create C processor
	cName := C.CString(config.Name)
	defer C.free(unsafe.Pointer(cName))

	ep.cptr = C.create_processor_go(
		cName,
		C.size_t(config.MaxQueueSize),
		C.bool(config.EnableLogging),
		// WARNING: uintptr cast used only as opaque ID, not dereferenced in C.
		// This is safe because we use it only for Go-side map lookup.
		// would use cgo.Handle in production code.
		unsafe.Pointer(uintptr(callbackID)),
	)

	if ep.cptr == nil {
		callbackMu.Lock()
		delete(callbackMap, callbackID)
		callbackMu.Unlock()
		return nil, fmt.Errorf("failed to create processor")
	}

	// Set finalizer to ensure cleanup
	runtime.SetFinalizer(ep, (*EventProcessor).finalize)

	ep.logger.Info("Event processor created",
		zap.String("name", config.Name),
		zap.Int("maxQueueSize", config.MaxQueueSize))

	return ep, nil
}

// Start starts the processor
func (ep *EventProcessor) Start() error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.closed {
		return fmt.Errorf("processor is closed")
	}

	C.event_processor_start(ep.cptr)
	return nil
}

// Stop stops the processor
func (ep *EventProcessor) Stop() error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.closed {
		return fmt.Errorf("processor is closed")
	}

	C.event_processor_stop(ep.cptr)
	return nil
}

// Push adds an event to the queue
func (ep *EventProcessor) Push(event Event) error {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return fmt.Errorf("processor is closed")
	}

	cSource := C.CString(event.Source)
	defer C.free(unsafe.Pointer(cSource))

	var dataPtr unsafe.Pointer
	if len(event.Data) > 0 {
		dataPtr = unsafe.Pointer(&event.Data[0])
	}

	success := C.event_processor_push(
		ep.cptr,
		C.event_type_t(event.Type),
		cSource,
		dataPtr,
		C.size_t(len(event.Data)),
	)

	if !success {
		return fmt.Errorf("failed to push event")
	}

	return nil
}

// Process processes a single event
func (ep *EventProcessor) Process() {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return
	}

	C.event_processor_process(ep.cptr)
}

// ProcessAll processes all queued events
func (ep *EventProcessor) ProcessAll() {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return
	}

	C.event_processor_process_all(ep.cptr)
}

// QueueSize returns the current queue size
func (ep *EventProcessor) QueueSize() int {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return 0
	}

	return int(C.event_processor_queue_size(ep.cptr))
}

// EventsProcessed returns total events processed
func (ep *EventProcessor) EventsProcessed() int {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return 0
	}

	return int(C.event_processor_events_processed(ep.cptr))
}

// State returns the current processor state
func (ep *EventProcessor) State() string {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if ep.closed {
		return "CLOSED"
	}

	return C.GoString(C.event_processor_get_state(ep.cptr))
}

// Close closes the processor and frees resources
func (ep *EventProcessor) Close() error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.closed {
		return nil
	}

	ep.closed = true

	// Clean up C resources
	if ep.cptr != nil {
		C.event_processor_destroy(ep.cptr)
		ep.cptr = nil
	}

	// Remove from callback map
	callbackMu.Lock()
	for id, proc := range callbackMap {
		if proc == ep {
			delete(callbackMap, id)
			break
		}
	}
	callbackMu.Unlock()

	ep.logger.Info("Event processor closed",
		zap.String("name", ep.config.Name))

	return nil
}

// finalize is called by GC if Close wasn't called
func (ep *EventProcessor) finalize() {
	if !ep.closed {
		ep.Close()
	}
}
