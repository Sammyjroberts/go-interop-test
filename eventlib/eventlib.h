// eventlib.h - Simple event processing library with callbacks
#ifndef EVENTLIB_H
#define EVENTLIB_H

#include <stdbool.h>
#include <stddef.h>

// Forward declarations
typedef struct event_processor event_processor_t;

// Event types
typedef enum {
  EVENT_TYPE_DATA,
  EVENT_TYPE_CONNECT,
  EVENT_TYPE_DISCONNECT,
  EVENT_TYPE_ERROR
} event_type_t;

// Event structure
typedef struct {
  event_type_t type;
  const char *source;
  const void *data;
  size_t data_len;
} event_t;

// Callback function types (these are your side effects)
typedef void (*on_event_cb)(const event_t *event, void *user_data);
typedef void (*on_log_cb)(const char *level, const char *message,
                          void *user_data);
typedef bool (*on_filter_cb)(const event_t *event, void *user_data);
typedef void (*on_state_change_cb)(const char *old_state, const char *new_state,
                                   void *user_data);

// Configuration structure
typedef struct {
  // Basic configuration
  const char *name;
  size_t max_queue_size;
  bool enable_logging;

  // Callback functions
  on_event_cb on_event;
  on_log_cb on_log;
  on_filter_cb on_filter; // Return false to drop event
  on_state_change_cb on_state_change;

  // User data passed to callbacks
  void *user_data;
} event_config_t;

// API Functions

// Create and destroy processor
event_processor_t *event_processor_create(const event_config_t *config);
void event_processor_destroy(event_processor_t *processor);

bool event_processor_push(event_processor_t *processor, event_type_t type,
                          const char *source, const void *data,
                          size_t data_len);

void event_processor_process(event_processor_t *processor);
void event_processor_process_all(event_processor_t *processor);

// State management
const char *event_processor_get_state(const event_processor_t *processor);
size_t event_processor_queue_size(const event_processor_t *processor);
size_t event_processor_events_processed(const event_processor_t *processor);

// Control functions
void event_processor_start(event_processor_t *processor);
void event_processor_stop(event_processor_t *processor);
void event_processor_clear_queue(event_processor_t *processor);

#endif // EVENTLIB_H