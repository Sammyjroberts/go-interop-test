package eventlib

/*
#include "../eventlib/eventlib.h"
*/
import "C"
import (
	"sync"
	"unsafe"

	"go.uber.org/zap"
)

// Global map to store processor references for callbacks
var (
	callbackMap    = make(map[int]*EventProcessor)
	callbackMu     sync.RWMutex
	nextCallbackID int
)

// getProcessor retrieves processor from callback ID
func getProcessor(userData unsafe.Pointer) *EventProcessor {
	id := int(uintptr(userData))
	callbackMu.RLock()
	defer callbackMu.RUnlock()
	return callbackMap[id]
}

//export goHandleEvent
func goHandleEvent(eventPtr unsafe.Pointer, userData unsafe.Pointer) {
	ep := getProcessor(userData)
	if ep == nil || ep.handlers.OnEvent == nil {
		return
	}

	// Convert C event to Go event
	cEvent := (*C.event_t)(eventPtr)
	event := Event{
		Type:   EventType(cEvent._type),
		Source: C.GoString(cEvent.source),
	}

	if cEvent.data != nil && cEvent.data_len > 0 {
		event.Data = C.GoBytes(cEvent.data, C.int(cEvent.data_len))
	}

	// Call handler with recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				ep.logger.Error("Panic in event handler",
					zap.Any("panic", r),
					zap.String("event_type", event.Type.String()))
			}
		}()
		ep.handlers.OnEvent(event)
	}()
}

//export goHandleLog
func goHandleLog(levelPtr unsafe.Pointer, messagePtr unsafe.Pointer, userData unsafe.Pointer) {
	ep := getProcessor(userData)
	if ep == nil {
		return
	}

	level := C.GoString((*C.char)(levelPtr))
	message := C.GoString((*C.char)(messagePtr))

	// Map C log levels to zap
	switch level {
	case "DEBUG":
		ep.logger.Debug(message)
	case "INFO":
		ep.logger.Info(message)
	case "WARN":
		ep.logger.Warn(message)
	case "ERROR":
		ep.logger.Error(message)
	default:
		ep.logger.Info(message, zap.String("level", level))
	}
}

//export goHandleFilter
func goHandleFilter(eventPtr unsafe.Pointer, userData unsafe.Pointer) C.int {
	ep := getProcessor(userData)
	if ep == nil || ep.handlers.OnFilter == nil {
		return 1 // Default: don't filter
	}

	// Convert C event to Go event
	cEvent := (*C.event_t)(eventPtr)
	event := Event{
		Type:   EventType(cEvent._type),
		Source: C.GoString(cEvent.source),
	}

	if cEvent.data != nil && cEvent.data_len > 0 {
		event.Data = C.GoBytes(cEvent.data, C.int(cEvent.data_len))
	}

	// Call filter with recovery
	allow := true
	func() {
		defer func() {
			if r := recover(); r != nil {
				ep.logger.Error("Panic in filter handler",
					zap.Any("panic", r))
				allow = true // Default to allowing on error
			}
		}()
		allow = ep.handlers.OnFilter(event)
	}()

	if allow {
		return 1
	}
	return 0
}

//export goHandleStateChange
func goHandleStateChange(oldStatePtr unsafe.Pointer, newStatePtr unsafe.Pointer, userData unsafe.Pointer) {
	ep := getProcessor(userData)
	if ep == nil || ep.handlers.OnStateChange == nil {
		return
	}

	oldState := C.GoString((*C.char)(oldStatePtr))
	newState := C.GoString((*C.char)(newStatePtr))

	// Call handler with recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				ep.logger.Error("Panic in state change handler",
					zap.Any("panic", r))
			}
		}()
		ep.handlers.OnStateChange(oldState, newState)
	}()
}
