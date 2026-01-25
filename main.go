package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "gpustat"
)

var (
	// Version is set via ldflags during build
	version = "dev"

	// Command line flags
	listenAddress  = flag.String("web.listen-address", ":9101", "Address to listen on for web interface and telemetry")
	metricsPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	gpustatPath    = flag.String("gpustat.path", "gpustat", "Path to gpustat binary")
	scrapeInterval = flag.Duration("scrape.interval", 30*time.Second, "Interval between gpustat scrapes")

	// Track previous metric label sets for cleanup
	previousUserMemoryLabels    = make(map[string]bool)
	previousProcessMemoryLabels = make(map[string]bool)

	// Prometheus metrics
	gpuTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "temperature_celsius",
			Help:      "GPU temperature in Celsius",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "utilization_percent",
			Help:      "GPU utilization percentage",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuMemoryUsed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_used_megabytes",
			Help:      "GPU memory used in megabytes",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuMemoryTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_total_megabytes",
			Help:      "GPU memory total in megabytes",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuMemoryUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory_utilization_percent",
			Help:      "GPU memory utilization percentage",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuProcessCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "process_count",
			Help:      "Number of processes running on GPU",
		},
		[]string{"hostname", "gpu_index", "gpu_name"},
	)

	gpuUserMemory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "user_memory_megabytes",
			Help:      "Total memory used by user on GPU",
		},
		[]string{"hostname", "gpu_index", "gpu_name", "username"},
	)

	gpuProcessMemory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "process_memory_megabytes",
			Help:      "Memory used by process on GPU",
		},
		[]string{"hostname", "gpu_index", "gpu_name", "username", "process_memory"},
	)

	driverVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nvidia",
			Name:      "driver_info",
			Help:      "NVIDIA driver version info",
		},
		[]string{"hostname", "version"},
	)

	scrapeSuccess = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scrape_success",
			Help:      "Whether the last scrape was successful",
		},
	)

	scrapeDuration = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scrape_duration_seconds",
			Help:      "Duration of the last scrape in seconds",
		},
	)
)

// GPUInfo represents information about a single GPU
type GPUInfo struct {
	Index       string
	Name        string
	Temperature float64
	Utilization float64
	MemoryUsed  float64
	MemoryTotal float64
	Processes   []ProcessInfo
}

// ProcessInfo represents a process running on a GPU
type ProcessInfo struct {
	Username string
	Memory   float64
}

// GPUStatOutput represents the parsed output of gpustat command
type GPUStatOutput struct {
	Hostname      string
	DriverVersion string
	GPUs          []GPUInfo
}

func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(gpuTemperature)
	prometheus.MustRegister(gpuUtilization)
	prometheus.MustRegister(gpuMemoryUsed)
	prometheus.MustRegister(gpuMemoryTotal)
	prometheus.MustRegister(gpuMemoryUtilization)
	prometheus.MustRegister(gpuProcessCount)
	prometheus.MustRegister(gpuUserMemory)
	prometheus.MustRegister(gpuProcessMemory)
	prometheus.MustRegister(driverVersion)
	prometheus.MustRegister(scrapeSuccess)
	prometheus.MustRegister(scrapeDuration)
}

// parseGPUStatOutput parses the output of gpustat command
func parseGPUStatOutput(output string) (*GPUStatOutput, error) {
	result := &GPUStatOutput{}
	scanner := bufio.NewScanner(strings.NewReader(output))

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if lineNum == 1 {
			// First line: hostname and driver version
			// Format: "hostname    date    driver_version"
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				result.Hostname = parts[0]
			}
			if len(parts) >= 5 {
				result.DriverVersion = parts[len(parts)-1]
			}
			continue
		}

		// GPU lines start with [N]
		if !strings.HasPrefix(line, "[") {
			continue
		}

		gpu, err := parseGPULine(line)
		if err != nil {
			log.Printf("Warning: failed to parse GPU line %d: %v", lineNum, err)
			continue
		}

		result.GPUs = append(result.GPUs, gpu)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading gpustat output: %w", err)
	}

	return result, nil
}

// parseGPULine parses a single GPU line from gpustat output
func parseGPULine(line string) (GPUInfo, error) {
	gpu := GPUInfo{}

	// Extract GPU index [N]
	indexRe := regexp.MustCompile(`^\[(\d+)\]`)
	if match := indexRe.FindStringSubmatch(line); len(match) > 1 {
		gpu.Index = match[1]
	}

	// Split by | to get different sections
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return gpu, fmt.Errorf("invalid GPU line format")
	}

	// Part 0: GPU name
	namePart := strings.TrimSpace(parts[0])
	// Remove the [N] prefix
	namePart = indexRe.ReplaceAllString(namePart, "")
	gpu.Name = strings.TrimSpace(namePart)

	// Part 1: Temperature and Utilization
	// Format: "49°C,   0 %" or "49'C,   0 %"
	tempUtilPart := strings.TrimSpace(parts[1])
	tempUtilRe := regexp.MustCompile(`(\d+)[°']C,\s*(\d+)\s*%`)
	if match := tempUtilRe.FindStringSubmatch(tempUtilPart); len(match) > 2 {
		if temp, err := strconv.ParseFloat(match[1], 64); err == nil {
			gpu.Temperature = temp
		}
		if util, err := strconv.ParseFloat(match[2], 64); err == nil {
			gpu.Utilization = util
		}
	}

	// Part 2: Memory usage
	// Format: "  1871 / 97887 MB"
	memPart := strings.TrimSpace(parts[2])
	memRe := regexp.MustCompile(`(\d+)\s*/\s*(\d+)\s*MB`)
	if match := memRe.FindStringSubmatch(memPart); len(match) > 2 {
		if used, err := strconv.ParseFloat(match[1], 64); err == nil {
			gpu.MemoryUsed = used
		}
		if total, err := strconv.ParseFloat(match[2], 64); err == nil {
			gpu.MemoryTotal = total
		}
	}

	// Part 3 (if exists): Processes
	// Format: "username(1224M)"
	if len(parts) > 3 {
		processesPart := strings.TrimSpace(parts[3])
		gpu.Processes = parseProcesses(processesPart)
	}

	return gpu, nil
}

// parseProcesses parses the processes part of a GPU line
// Format: "user1(123M) user2(456M)"
func parseProcesses(processesStr string) []ProcessInfo {
	var processes []ProcessInfo

	if processesStr == "" {
		return processes
	}

	// Match pattern: username(memoryM)
	processRe := regexp.MustCompile(`(\w+)\((\d+)M\)`)
	matches := processRe.FindAllStringSubmatch(processesStr, -1)

	for _, match := range matches {
		if len(match) > 2 {
			username := match[1]
			if memory, err := strconv.ParseFloat(match[2], 64); err == nil {
				processes = append(processes, ProcessInfo{
					Username: username,
					Memory:   memory,
				})
			}
		}
	}

	return processes
}

// collectMetrics runs gpustat and updates Prometheus metrics
func collectMetrics() error {
	start := time.Now()

	// Run gpustat command
	cmd := exec.Command(*gpustatPath)
	output, err := cmd.Output()
	if err != nil {
		scrapeSuccess.Set(0)
		return fmt.Errorf("failed to execute gpustat: %w", err)
	}

	// Parse output
	stats, err := parseGPUStatOutput(string(output))
	if err != nil {
		scrapeSuccess.Set(0)
		return fmt.Errorf("failed to parse gpustat output: %w", err)
	}

	// Reset basic GPU metrics (these are always set for all GPUs)
	gpuTemperature.Reset()
	gpuUtilization.Reset()
	gpuMemoryUsed.Reset()
	gpuMemoryTotal.Reset()
	gpuMemoryUtilization.Reset()
	gpuProcessCount.Reset()
	driverVersion.Reset()

	// Track current label sets for user and process metrics
	currentUserMemoryLabels := make(map[string]bool)
	currentProcessMemoryLabels := make(map[string]bool)

	// Update driver version
	if stats.DriverVersion != "" {
		driverVersion.WithLabelValues(stats.Hostname, stats.DriverVersion).Set(1)
	}

	// Update GPU metrics
	for _, gpu := range stats.GPUs {
		labels := prometheus.Labels{
			"hostname":  stats.Hostname,
			"gpu_index": gpu.Index,
			"gpu_name":  gpu.Name,
		}

		gpuTemperature.With(labels).Set(gpu.Temperature)
		gpuUtilization.With(labels).Set(gpu.Utilization)
		gpuMemoryUsed.With(labels).Set(gpu.MemoryUsed)
		gpuMemoryTotal.With(labels).Set(gpu.MemoryTotal)

		// Calculate memory utilization percentage
		if gpu.MemoryTotal > 0 {
			memUtil := (gpu.MemoryUsed / gpu.MemoryTotal) * 100
			gpuMemoryUtilization.With(labels).Set(memUtil)
		}

		// Process count
		gpuProcessCount.With(labels).Set(float64(len(gpu.Processes)))

		// Aggregate memory by user
		userMemory := make(map[string]float64)
		for _, proc := range gpu.Processes {
			userMemory[proc.Username] += proc.Memory

			// Individual process memory
			procLabelKey := fmt.Sprintf("%s|%s|%s|%s|%.0fM", stats.Hostname, gpu.Index, gpu.Name, proc.Username, proc.Memory)
			currentProcessMemoryLabels[procLabelKey] = true

			procLabels := prometheus.Labels{
				"hostname":       stats.Hostname,
				"gpu_index":      gpu.Index,
				"gpu_name":       gpu.Name,
				"username":       proc.Username,
				"process_memory": fmt.Sprintf("%.0fM", proc.Memory),
			}
			gpuProcessMemory.With(procLabels).Set(proc.Memory)
		}

		// User memory totals
		for username, memory := range userMemory {
			userLabelKey := fmt.Sprintf("%s|%s|%s|%s", stats.Hostname, gpu.Index, gpu.Name, username)
			currentUserMemoryLabels[userLabelKey] = true

			userLabels := prometheus.Labels{
				"hostname":  stats.Hostname,
				"gpu_index": gpu.Index,
				"gpu_name":  gpu.Name,
				"username":  username,
			}
			gpuUserMemory.With(userLabels).Set(memory)
		}
	}

	// Delete stale user memory metrics
	for labelKey := range previousUserMemoryLabels {
		if !currentUserMemoryLabels[labelKey] {
			// Parse the label key back into label values
			parts := strings.Split(labelKey, "|")
			if len(parts) == 4 {
				deleted := gpuUserMemory.DeleteLabelValues(parts[0], parts[1], parts[2], parts[3])
				if deleted {
					log.Printf("Deleted stale user memory metric: hostname=%s gpu_index=%s gpu_name=%s username=%s",
						parts[0], parts[1], parts[2], parts[3])
				}
			}
		}
	}

	// Delete stale process memory metrics
	for labelKey := range previousProcessMemoryLabels {
		if !currentProcessMemoryLabels[labelKey] {
			// Parse the label key back into label values
			parts := strings.Split(labelKey, "|")
			if len(parts) == 5 {
				deleted := gpuProcessMemory.DeleteLabelValues(parts[0], parts[1], parts[2], parts[3], parts[4])
				if deleted {
					log.Printf("Deleted stale process memory metric: hostname=%s gpu_index=%s gpu_name=%s username=%s process_memory=%s",
						parts[0], parts[1], parts[2], parts[3], parts[4])
				}
			}
		}
	}

	// Update the previous label sets for next scrape
	previousUserMemoryLabels = currentUserMemoryLabels
	previousProcessMemoryLabels = currentProcessMemoryLabels

	duration := time.Since(start).Seconds()
	scrapeDuration.Set(duration)
	scrapeSuccess.Set(1)

	log.Printf("Successfully scraped %d GPUs from %s in %.3fs", len(stats.GPUs), stats.Hostname, duration)
	return nil
}

// metricsCollector runs collectMetrics at the specified interval
func metricsCollector(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect metrics immediately on startup
	if err := collectMetrics(); err != nil {
		log.Printf("Error collecting metrics: %v", err)
	}

	for range ticker.C {
		if err := collectMetrics(); err != nil {
			log.Printf("Error collecting metrics: %v", err)
		}
	}
}

func main() {
	flag.Parse()

	// Check if gpustat is available
	if _, err := exec.LookPath(*gpustatPath); err != nil {
		log.Fatalf("gpustat command not found. Please install it: sudo apt install gpustat")
	}

	// Start metrics collector in background
	go metricsCollector(*scrapeInterval)

	// Setup HTTP handlers
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html>
<head><title>GPUstat Exporter</title></head>
<body>
<h1>GPUstat Exporter</h1>
<p><a href='%s'>Metrics</a></p>
<h2>Build Info</h2>
<ul>
<li>Version: %s</li>
<li>Scrape Interval: %s</li>
<li>GPUstat Path: %s</li>
</ul>
</body>
</html>`, *metricsPath, version, *scrapeInterval, *gpustatPath)
	})

	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "%s\n", version)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "OK")
	})

	// Start HTTP server
	log.Printf("Starting gpustat-exporter version %s on %s", version, *listenAddress)
	log.Printf("Metrics available at %s%s", *listenAddress, *metricsPath)
	log.Printf("Scrape interval: %s", *scrapeInterval)

	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}
