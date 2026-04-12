package main

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration parameters for the TTS pipeline
type Config struct {
	// TTS Backend
	TTSLoadBalancerURL string
	TTSDirectURL       string
	TTSHealthEndpoint  string

	// Daemon Settings
	DaemonPort         string
	DaemonHost         string
	DaemonReadTimeout  time.Duration
	DaemonWriteTimeout time.Duration
	DaemonIdleTimeout  time.Duration

	// Audio Settings
	DefaultSpeaker string
	HALSpeaker     string
	DragThreshold  time.Duration
	MaxRetries     int
	WarmupCount    int

	// HTTP Performance
	HTTPTimeout               time.Duration
	HTTPResponseHeaderTimeout time.Duration
	MaxIdleConnections        int
	MaxIdleConnectionsPerHost int
	IdleConnectionTimeout     time.Duration

	// File System
	TmpAudioDir string
	LogLevel    string
	LogFile     string

	// Development
	EnablePerformanceMetrics bool
	EnableDebugLogging       bool

	// Kubernetes
	TTSNamespace      string
	TTSDeploymentName string
	KubectlContext    string
}

// LoadConfig loads configuration from environment variables with secure defaults
func LoadConfig() *Config {
	return &Config{
		// TTS Backend - use environment or secure defaults
		TTSLoadBalancerURL: getEnvString("TTS_LOADBALANCER_URL", "http://localhost:5002/api/tts"),
		TTSDirectURL:       getEnvString("TTS_DIRECT_URL", "http://localhost:5002/api/tts"),
		TTSHealthEndpoint:  getEnvString("TTS_HEALTH_ENDPOINT", "http://localhost:5002/health"),

		// Daemon Settings
		DaemonPort:         getEnvString("DAEMON_PORT", "8091"),
		DaemonHost:         getEnvString("DAEMON_HOST", "localhost"),
		DaemonReadTimeout:  getEnvDuration("DAEMON_READ_TIMEOUT", 2*time.Second),
		DaemonWriteTimeout: getEnvDuration("DAEMON_WRITE_TIMEOUT", 3*time.Second),
		DaemonIdleTimeout:  getEnvDuration("DAEMON_IDLE_TIMEOUT", 60*time.Second),

		// Audio Settings
		DefaultSpeaker: getEnvString("DEFAULT_SPEAKER", "p245"),
		HALSpeaker:     getEnvString("HAL_SPEAKER", "p254"),
		DragThreshold:  getEnvDuration("DRAG_THRESHOLD_SECONDS", 3*time.Second),
		MaxRetries:     getEnvInt("MAX_RETRIES", 2),
		WarmupCount:    getEnvInt("WARMUP_CONNECTION_COUNT", 5),

		// HTTP Performance
		HTTPTimeout:               getEnvDuration("HTTP_TIMEOUT_SECONDS", 30*time.Second),
		HTTPResponseHeaderTimeout: getEnvDuration("HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS", 10*time.Second),
		MaxIdleConnections:        getEnvInt("MAX_IDLE_CONNECTIONS", 100),
		MaxIdleConnectionsPerHost: getEnvInt("MAX_IDLE_CONNECTIONS_PER_HOST", 20),
		IdleConnectionTimeout:     getEnvDuration("IDLE_CONNECTION_TIMEOUT_SECONDS", 300*time.Second),

		// File System
		TmpAudioDir: getEnvString("TMP_AUDIO_DIR", "/tmp/tts_pipeline_cache"),
		LogLevel:    getEnvString("LOG_LEVEL", "INFO"),
		LogFile:     getEnvString("LOG_FILE", "/var/log/tts_pipeline.log"),

		// Development
		EnablePerformanceMetrics: getEnvBool("ENABLE_PERFORMANCE_METRICS", true),
		EnableDebugLogging:       getEnvBool("ENABLE_DEBUG_LOGGING", false),

		// Kubernetes
		TTSNamespace:      getEnvString("TTS_NAMESPACE", "tts"),
		TTSDeploymentName: getEnvString("TTS_DEPLOYMENT_NAME", "coqui-tts"),
		KubectlContext:    getEnvString("KUBECTL_CONTEXT", "default"),
	}
}

// Helper functions for environment variable parsing

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		// Try parsing as seconds first
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
		// Try parsing as duration string (e.g., "30s", "5m")
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// GetDaemonURL returns the complete daemon URL for clients
func (c *Config) GetDaemonURL() string {
	return "http://" + c.DaemonHost + ":" + c.DaemonPort
}

// GetServerPort returns the port with colon prefix for server binding
func (c *Config) GetServerPort() string {
	return ":" + c.DaemonPort
}

// Validation methods

// Validate checks all configuration parameters for validity
func (c *Config) Validate() error {
	// Add validation logic here if needed
	// For example: check URL formats, port ranges, etc.
	return nil
}
