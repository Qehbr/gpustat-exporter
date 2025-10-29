FROM golang:1.21-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gpustat-exporter .

# Final stage
FROM python:3.11-slim

# Install gpustat via pip
RUN pip install --no-cache-dir gpustat

# Copy the binary from builder
COPY --from=builder /build/gpustat-exporter /usr/local/bin/gpustat-exporter

# Create non-root user
RUN useradd -r -u 1000 exporter

USER exporter

EXPOSE 9101

ENTRYPOINT ["/usr/local/bin/gpustat-exporter"]
CMD ["--web.listen-address=:9101", "--web.telemetry-path=/metrics"]
