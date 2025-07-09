FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev make

# Set working directory
WORKDIR /build

# Copy go workspace file
COPY go.work go.work

# Copy all modules
COPY eventlib/ ./eventlib/
COPY eventlibgo/ ./eventlibgo/
COPY eventlibserver/ ./eventlibserver/

# Build C library
WORKDIR /build/eventlib
RUN gcc -c -fPIC -O2 eventlib.c -o eventlib.o && \
  ar rcs libeventlib.a eventlib.o && \
  rm eventlib.o

# Download Go dependencies using workspace
WORKDIR /build
RUN go mod download all

# Build the server
WORKDIR /build/eventlibserver
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o eventlib-server .

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 appuser && \
  adduser -D -u 1000 -G appuser appuser

# Copy binary from builder
COPY --from=builder /build/eventlibserver/eventlib-server /usr/local/bin/eventlib-server

# Change ownership
RUN chown appuser:appuser /usr/local/bin/eventlib-server

# Use non-root user
USER appuser

# Expose ports
EXPOSE 8080 9090


# Run the server
ENTRYPOINT ["eventlib-server"]