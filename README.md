# GPUstat Exporter

Prometheus exporter for NVIDIA GPU statistics using `gpustat`.

## Prerequisites

- `gpustat` installed: `sudo apt install gpustat`
- NVIDIA GPU with drivers installed

## Installation

Download the binary from [releases](https://github.com/qehbr/gpustat-exporter/releases):

```bash
wget https://github.com/qehbr/gpustat-exporter/releases/latest/download/gpustat-exporter-linux-amd64
chmod +x gpustat-exporter-linux-amd64
sudo mv gpustat-exporter-linux-amd64 /usr/local/bin/gpustat-exporter
```

## Usage

### Run manually

```bash
gpustat-exporter --web.listen-address=:9101 --web.telemetry-path=/metrics
```

### Run as systemd service

```bash
# Download the service file
sudo wget -O /etc/systemd/system/gpustat-exporter.service https://raw.githubusercontent.com/qehbr/gpustat-exporter/main/gpustat-exporter.service

# Reload systemd and enable the service
sudo systemctl daemon-reload
sudo systemctl enable gpustat-exporter
sudo systemctl start gpustat-exporter

# Check status
sudo systemctl status gpustat-exporter

# View logs
sudo journalctl -u gpustat-exporter -f
```

### Run with Docker

Requires [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) for GPU access.

```bash
# Build the image
docker build -t gpustat-exporter .

# Run the container
docker run -d \
  --name gpustat-exporter \
  --restart unless-stopped \
  -p 9101:9101 \
  --gpus all \
  gpustat-exporter
```

### Flags

- `--web.listen-address` - Address to listen on (default: `:9101`)
- `--web.telemetry-path` - Metrics path (default: `/metrics`)
- `--gpustat.path` - Path to gpustat binary (default: `gpustat`)
- `--scrape.interval` - Scrape interval (default: `30s`)

## Metrics

- `gpustat_temperature_celsius` - GPU temperature
- `gpustat_utilization_percent` - GPU utilization
- `gpustat_memory_used_megabytes` - GPU memory used
- `gpustat_memory_total_megabytes` - GPU memory total
- `gpustat_memory_utilization_percent` - GPU memory utilization
- `gpustat_process_count` - Number of processes on GPU
- `gpustat_user_memory_megabytes` - Memory used by user
- `nvidia_driver_info` - NVIDIA driver version

## Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'gpustat'
    static_configs:
      - targets: ['localhost:9101']
```

## Grafana Dashboard

A pre-built Grafana dashboard is available in [grafana-dashboard.json](grafana-dashboard.json). It includes:

- GPU utilization and memory overview
- GPU status table with temperature and process info
- Usage by user
- Time-series graphs for utilization, memory, temperature, and process count

**Import instructions:**
1. Open Grafana
2. Go to Dashboards → Import
3. Upload `grafana-dashboard.json` or paste its contents
4. Select your Prometheus datasource

## Build from Source

Requires Go 1.21 or later.

```bash
# Install Go if needed
# https://go.dev/doc/install

# Clone and build
git clone https://github.com/qehbr/gpustat-exporter.git
cd gpustat-exporter
make build

# Binary will be created as gpustat-exporter
./gpustat-exporter
```
