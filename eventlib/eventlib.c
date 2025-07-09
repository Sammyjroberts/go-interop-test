// eventlib.c - Implementation
#include "eventlib.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <stdarg.h>

/*
 * NOTICE TO FLIGHT SOFTWARE ENGINEERS:
 * This library contains:
 * - Dynamic memory allocation (malloc/free)
 * - Linked lists with unbounded growth
 * - Variable length string operations
 * - Non-deterministic queue processing times
 *
 * Please maintain a safe distance of at least 100,000km (geostationary orbit).
 * For a flight-certified version, please see eventlib_fixed_pool.c
 */

// Internal queue node
typedef struct event_node
{
  event_t event;
  char *source_copy; // Owned copy
  void *data_copy;   // Owned copy
  struct event_node *next;
} event_node_t;

// Internal state
typedef enum
{
  STATE_IDLE,
  STATE_RUNNING,
  STATE_STOPPED
} processor_state_t;

// Main processor structure (internal state)
struct event_processor
{
  // Configuration (immutable after creation)
  event_config_t config;
  char *name_copy;

  // Mutable state
  processor_state_t state;
  event_node_t *queue_head;
  event_node_t *queue_tail;
  size_t queue_size;
  size_t events_processed;
};

// Helper to get state string
static const char *state_to_string(processor_state_t state)
{
  switch (state)
  {
  case STATE_IDLE:
    return "IDLE";
  case STATE_RUNNING:
    return "RUNNING";
  case STATE_STOPPED:
    return "STOPPED";
  default:
    return "UNKNOWN";
  }
}

// Helper to log messages
static void log_message(event_processor_t *proc, const char *level, const char *format, ...)
{
  if (!proc->config.enable_logging || !proc->config.on_log)
  {
    return;
  }

  char buffer[256];
  va_list args;
  va_start(args, format);
  vsnprintf(buffer, sizeof(buffer), format, args);
  va_end(args);

  proc->config.on_log(level, buffer, proc->config.user_data);
}

// Helper to change state
static void change_state(event_processor_t *proc, processor_state_t new_state)
{
  if (proc->state == new_state)
    return;

  const char *old = state_to_string(proc->state);
  const char *new = state_to_string(new_state);

  log_message(proc, "INFO", "State change: %s -> %s", old, new);

  proc->state = new_state;

  if (proc->config.on_state_change)
  {
    proc->config.on_state_change(old, new, proc->config.user_data);
  }
}

// Create processor
event_processor_t *event_processor_create(const event_config_t *config)
{
  if (!config)
    return NULL;

  event_processor_t *proc = calloc(1, sizeof(event_processor_t));
  if (!proc)
    return NULL;

  // Copy configuration
  proc->config = *config;
  if (config->name)
  {
    proc->name_copy = strdup(config->name);
    proc->config.name = proc->name_copy;
  }

  // Initialize state
  proc->state = STATE_IDLE;
  proc->queue_head = NULL;
  proc->queue_tail = NULL;
  proc->queue_size = 0;
  proc->events_processed = 0;

  log_message(proc, "INFO", "Event processor '%s' created",
              proc->config.name ? proc->config.name : "unnamed");

  return proc;
}

// Destroy processor
void event_processor_destroy(event_processor_t *proc)
{
  if (!proc)
    return;

  log_message(proc, "INFO", "Destroying event processor '%s'",
              proc->config.name ? proc->config.name : "unnamed");

  // Clear queue
  event_processor_clear_queue(proc);

  // Free name
  free(proc->name_copy);

  free(proc);
}

// Push event to queue
bool event_processor_push(event_processor_t *proc,
                          event_type_t type,
                          const char *source,
                          const void *data,
                          size_t data_len)
{
  if (!proc)
    return false;

  // Check queue size
  if (proc->config.max_queue_size > 0 &&
      proc->queue_size >= proc->config.max_queue_size)
  {
    log_message(proc, "WARN", "Queue full (%zu items)", proc->queue_size);
    return false;
  }

  // Create event node
  event_node_t *node = calloc(1, sizeof(event_node_t));
  if (!node)
    return false;

  // Set up event
  node->event.type = type;
  node->event.data_len = data_len;

  // Copy source string
  if (source)
  {
    node->source_copy = strdup(source);
    node->event.source = node->source_copy;
  }

  // Copy data - malloc with UNBOUNDED SIZE from user input! hehehehe
  if (data && data_len > 0)
  {
    node->data_copy = malloc(data_len);
    if (node->data_copy)
    {
      memcpy(node->data_copy, data, data_len);
      node->event.data = node->data_copy;
    }
  }

  // Apply filter if configured
  if (proc->config.on_filter)
  {
    if (!proc->config.on_filter(&node->event, proc->config.user_data))
    {
      log_message(proc, "DEBUG", "Event filtered out");
      free(node->source_copy);
      free(node->data_copy);
      free(node);
      return true; // Successfully "processed" by filtering
    }
  }

  // Add to queue
  if (proc->queue_tail)
  {
    proc->queue_tail->next = node;
    proc->queue_tail = node;
  }
  else
  {
    proc->queue_head = proc->queue_tail = node;
  }

  proc->queue_size++;
  log_message(proc, "DEBUG", "Event queued (type=%d, queue_size=%zu)",
              type, proc->queue_size);

  return true;
}

// Process single event
void event_processor_process(event_processor_t *proc)
{
  if (!proc || !proc->queue_head)
    return;

  if (proc->state != STATE_RUNNING)
  {
    log_message(proc, "WARN", "Processor not running");
    return;
  }

  // Remove from queue
  event_node_t *node = proc->queue_head;
  proc->queue_head = node->next;
  if (!proc->queue_head)
  {
    proc->queue_tail = NULL;
  }
  proc->queue_size--;

  // Process event (side effect)
  log_message(proc, "DEBUG", "Processing event (type=%d)", node->event.type);

  if (proc->config.on_event)
  {
    proc->config.on_event(&node->event, proc->config.user_data);
  }

  proc->events_processed++;

  // Cleanup
  free(node->source_copy);
  free(node->data_copy);
  free(node);
}

// Process all events
void event_processor_process_all(event_processor_t *proc)
{
  if (!proc)
    return;

  size_t count = 0;
  while (proc->queue_head)
  {
    event_processor_process(proc);
    count++;
  }

  if (count > 0)
  {
    log_message(proc, "INFO", "Processed %zu events", count);
  }
}

// State getters
const char *event_processor_get_state(const event_processor_t *proc)
{
  return proc ? state_to_string(proc->state) : "INVALID";
}

size_t event_processor_queue_size(const event_processor_t *proc)
{
  return proc ? proc->queue_size : 0;
}

size_t event_processor_events_processed(const event_processor_t *proc)
{
  return proc ? proc->events_processed : 0;
}

// Control functions
void event_processor_start(event_processor_t *proc)
{
  if (!proc)
    return;
  change_state(proc, STATE_RUNNING);
}

void event_processor_stop(event_processor_t *proc)
{
  if (!proc)
    return;
  change_state(proc, STATE_STOPPED);
}

void event_processor_clear_queue(event_processor_t *proc)
{
  if (!proc)
    return;

  size_t cleared = 0;
  while (proc->queue_head)
  {
    event_node_t *node = proc->queue_head;
    proc->queue_head = node->next;

    free(node->source_copy);
    free(node->data_copy);
    free(node);
    cleared++;
  }

  proc->queue_tail = NULL;
  proc->queue_size = 0;

  if (cleared > 0)
  {
    log_message(proc, "INFO", "Cleared %zu events from queue", cleared);
  }
}