# EventLib C/Go Interop Prototype

## Overview

This project demonstrates seamless **C/Go interoperation** to enable **shared event-processing logic** between Flight Software (FSW) and Ground Software (GSW) teams. At its core is a C library (`eventlib`) that delegates behavior through function pointers. These are hooked in via a Go wrapper (`eventlibgo`) and ultimately exposed via a REST API server (`eventlibserver`).

By pushing system-specific side effects (like logging, filtering, or output) into user-defined callbacks from Go, we make the C library portable across mission profiles—from satellites using a message bus to ground stations storing events in Redis.

This is a **strategic pattern for aerospace software**: domain-specific behavior lives in a high-level, ergonomic language (Go), while deterministic core logic remains in C.

---

## Architecture Diagram

The architecture consists of three main layers:

1. **eventlib (C)** – a portable, side-effect-free event queue processor that takes a config struct of function pointers.
2. **eventlibgo (Go)** – a CGo bridge that wraps Go-side logic (like logging and filtering) and exposes it to C by wrapping Go functions in exported C shims.
3. **eventlibserver (Go)** – exposes the eventlib processor as an HTTP server for real-time interaction and metrics.

```text
  +--------------------+            +---------------------+
  |   eventlibserver   |  HTTP API  | External systems    |
  |  (REST + Prometheus)+----------> (curl, Prometheus,  |
  +--------------------+            |  mission ops UI...) |
           |
           v
  +--------------------+    Wraps    +------------------+
  |    eventlibgo      +------------>   CGo Wrapper     |
  | (Go Callbacks +    |            | (callback shims) |
  |  Processor Config) |            +------------------+
           |
           v
  +----------------------------+
  |         eventlib          |
  |   (Pure C Event Engine)   |
  |   - Queue mgmt            |
  |   - Event dispatching     |
  |   - Callback hooks        |
  +----------------------------+
```

### Callback Shim Explained

Since C cannot directly call Go functions, we implement **C wrapper functions in Go using `import "C"` with `//export` directives**. These act as trampoline functions that C can invoke, which in turn call Go functions using user-provided pointers:

#### Example:

```go
//export goHandleEvent
func goHandleEvent(eventPtr unsafe.Pointer, userData unsafe.Pointer) {
    // Lookup Go handler by ID
    ep := getProcessor(userData)
    ep.handlers.OnEvent(event) // Actual Go callback
}

// In C section of Go file
static void c_handle_event(const event_t* event, void* user_data) {
    goHandleEvent((void*)event, user_data);
}

// Passed into C as part of config:
.on_event = c_handle_event
```

This mechanism is what enables the C library to remain pure and generic, while allowing domain-specific extensions from the Go or C side.


## How to Run

### Requirements

* Docker + Docker Compose

### Start the Server

```bash
docker-compose up --build
```

### Test With Curl

**Push a single event:**

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "type": 1,
    "source": "mission-ops",
    "data": "aGVsbG8gd29ybGQ="
  }'
```

**Check health:**

```bash
curl http://localhost:8080/api/v1/health
```

**Process events manually:**

```bash
curl -X POST http://localhost:8080/api/v1/process/all
```

---
**Queue Status:**

```bash
curl http://localhost:8080/api/v1/status
```

## Repo Layout

```
.
├── eventlib/             # Core C event library (portable logic)
│   ├── eventlib.h        # C API definition
│   ├── eventlib.c        # C implementation
├── eventlibgo/           # CGo bridge + Go wrappers and callback glue
├── eventlibserver/       # HTTP API around Go wrapper
│   └── main.go           # REST, metrics, queue introspection
├── go.work               # Go workspace for all modules
├── docker-compose.yaml   # Docker services
└── Dockerfile            # Multistage server image
```

---

## Innovation for Aerospace

This prototype isn't just a neat trick—it's a **serious enabler** of collaboration and long-term maintainability in space systems:

* **FSW and GSW can share the same tested logic**
* Custom side-effects like telemetry routing, persistent logging, and validation can be done without editing the C code
* **No duplication**: Customize behavior without rewriting or forking core code
* **System boundaries respected**: real-time systems stay lean, ground systems stay expressive

> *Shared core logic, specialized behavior—together at last.*

## Further Reading: CGo and Callback Handling

- [Go Runtime `cgo.Handle` Documentation](https://pkg.go.dev/runtime/cgo#Handle)
  Official Go documentation for `cgo.Handle`, which provides a safe way to pass Go values through C as opaque handles.


- [Go Blog: CGo — The Basics](https://go.dev/blog/cgo)
  Introductory blog post by the Go team explaining how `cgo` works under the hood.


---

## License

MIT License is available under the [MIT License](LICENSE).

---

## Authors

Turion Space ✦ GSW/FSW Team

Let's make space software interoperable, fast, and fun.
