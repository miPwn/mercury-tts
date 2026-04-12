package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Logger struct {
	*slog.Logger
	closeFn func(context.Context) error
}

type Options struct {
	Service      string
	StaticLabels map[string]string
	Logcity      LogcityConfig
}

type LogcityConfig struct {
	Enabled        bool
	Endpoint       string
	TenantID       string
	BufferSize     int
	BatchSize      int
	FlushInterval  time.Duration
	RequestTimeout time.Duration
}

type teeHandler struct {
	stdout slog.Handler
	sink   *asyncLogcitySink
}

type asyncLogcitySink struct {
	client        *http.Client
	endpoint      string
	tenantID      string
	labels        map[string]string
	buffer        chan logEntry
	flushInterval time.Duration
	batchSize     int
	stop          chan struct{}
	done          chan struct{}
	stopOnce      sync.Once
	stopped       atomic.Bool
	dropped       atomic.Int64
}

type logEntry struct {
	timestamp string
	line      string
}

type stdlibWriter struct {
	logger       *Logger
	defaultLevel slog.Level
}

func NewLoggerWithOptions(level slog.Level, opts Options) (*Logger, func(context.Context) error, error) {
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	handler := &teeHandler{stdout: stdout}
	closeFn := func(context.Context) error { return nil }

	if opts.Logcity.Enabled {
		sink, err := newAsyncLogcitySink(opts)
		if err != nil {
			return nil, nil, err
		}
		handler.sink = sink
		closeFn = sink.Close
	}

	logger := &Logger{
		Logger:  slog.New(handler),
		closeFn: closeFn,
	}
	return logger, closeFn, nil
}

func LoadOptionsFromEnv(service string) (Options, error) {
	hostname, _ := os.Hostname()
	labels := map[string]string{
		"service": service,
		"host":    strings.TrimSpace(hostname),
		"stack":   "halo",
	}
	if env := strings.TrimSpace(os.Getenv("LOGCITY_ENV")); env != "" {
		labels["env"] = env
	}

	cfg := LogcityConfig{
		Enabled:        getenvBool("LOGCITY_ENABLED", false),
		Endpoint:       strings.TrimSpace(os.Getenv("LOGCITY_ENDPOINT")),
		TenantID:       strings.TrimSpace(os.Getenv("LOGCITY_TENANT_ID")),
		BufferSize:     getenvInt("LOGCITY_BUFFER_SIZE", 2048),
		BatchSize:      getenvInt("LOGCITY_BATCH_SIZE", 64),
		FlushInterval:  time.Duration(getenvInt("LOGCITY_FLUSH_INTERVAL_MS", 750)) * time.Millisecond,
		RequestTimeout: time.Duration(getenvInt("LOGCITY_REQUEST_TIMEOUT_MS", 1500)) * time.Millisecond,
	}
	if cfg.Enabled {
		if cfg.Endpoint == "" {
			return Options{}, errors.New("LOGCITY_ENDPOINT is required when LOGCITY_ENABLED=true")
		}
		if cfg.TenantID == "" {
			return Options{}, errors.New("LOGCITY_TENANT_ID is required when LOGCITY_ENABLED=true")
		}
	}

	return Options{
		Service:      service,
		StaticLabels: labels,
		Logcity:      cfg,
	}, nil
}

func ParseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "trace", "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func NewStdlibWriter(logger *Logger, defaultLevel slog.Level) io.Writer {
	return &stdlibWriter{
		logger:       logger,
		defaultLevel: defaultLevel,
	}
}

func (l *Logger) Close(ctx context.Context) error {
	if l == nil || l.closeFn == nil {
		return nil
	}
	return l.closeFn(ctx)
}

func (w *stdlibWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(bytes.TrimSpace(p)))
	if line == "" || w.logger == nil {
		return len(p), nil
	}

	level := w.defaultLevel
	switch {
	case strings.HasPrefix(line, "ERROR:"), strings.HasPrefix(line, "FATAL:"), strings.HasPrefix(line, "PLAYBACK ERROR:"):
		level = slog.LevelError
	case strings.HasPrefix(line, "WARN:"), strings.HasPrefix(line, "WARNING:"), strings.HasPrefix(line, "DRAG:"):
		level = slog.LevelWarn
	case strings.HasPrefix(line, "DEBUG:"):
		level = slog.LevelDebug
	}

	switch level {
	case slog.LevelDebug:
		w.logger.Debug(line)
	case slog.LevelWarn:
		w.logger.Warn(line)
	case slog.LevelError:
		w.logger.Error(line)
	default:
		w.logger.Info(line)
	}
	return len(p), nil
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.stdout.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.stdout.Handle(ctx, record.Clone())
	if h.sink != nil {
		h.sink.Emit(record)
	}
	return err
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{stdout: h.stdout.WithAttrs(attrs), sink: h.sink}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{stdout: h.stdout.WithGroup(name), sink: h.sink}
}

func newAsyncLogcitySink(opts Options) (*asyncLogcitySink, error) {
	cfg := opts.Logcity
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 2048
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 64
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 750 * time.Millisecond
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 1500 * time.Millisecond
	}

	labels := map[string]string{}
	for key, value := range opts.StaticLabels {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		labels[key] = value
	}
	if labels["service"] == "" && strings.TrimSpace(opts.Service) != "" {
		labels["service"] = opts.Service
	}
	if labels["service"] == "" {
		labels["service"] = "unknown"
	}

	sink := &asyncLogcitySink{
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          16,
				MaxIdleConnsPerHost:   16,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: cfg.RequestTimeout,
			},
		},
		endpoint:      cfg.Endpoint,
		tenantID:      cfg.TenantID,
		labels:        labels,
		buffer:        make(chan logEntry, cfg.BufferSize),
		flushInterval: cfg.FlushInterval,
		batchSize:     cfg.BatchSize,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go sink.run()
	return sink, nil
}

func (s *asyncLogcitySink) Emit(record slog.Record) {
	if s == nil || s.stopped.Load() {
		return
	}
	payload, err := encodeRecord(record, s.labels["service"])
	if err != nil {
		fmt.Fprintf(os.Stderr, "logcity encode failed: %v\n", err)
		return
	}
	entry := logEntry{
		timestamp: strconv.FormatInt(record.Time.UTC().UnixNano(), 10),
		line:      string(payload),
	}
	select {
	case s.buffer <- entry:
	default:
		s.dropped.Add(1)
	}
}

func (s *asyncLogcitySink) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		close(s.stop)
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *asyncLogcitySink) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	batch := make([]logEntry, 0, s.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.flush(batch); err != nil {
			fmt.Fprintf(os.Stderr, "logcity flush failed: %v\n", err)
		}
		batch = batch[:0]
		if dropped := s.dropped.Swap(0); dropped > 0 {
			fmt.Fprintf(os.Stderr, "logcity dropped %d log entries due to full buffer\n", dropped)
		}
	}

	for {
		select {
		case entry := <-s.buffer:
			batch = append(batch, entry)
			if len(batch) >= s.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stop:
			for {
				select {
				case entry := <-s.buffer:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (s *asyncLogcitySink) flush(entries []logEntry) error {
	values := make([][]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, []string{entry.timestamp, entry.line})
	}
	payload := map[string]any{
		"streams": []any{
			map[string]any{
				"stream": s.labels,
				"values": values,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, s.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Scope-OrgID", s.tenantID)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func encodeRecord(record slog.Record, service string) ([]byte, error) {
	payload := map[string]any{
		"timestamp": record.Time.UTC().Format(time.RFC3339Nano),
		"level":     record.Level.String(),
		"message":   record.Message,
		"service":   service,
	}
	record.Attrs(func(attr slog.Attr) bool {
		attr.Value = attr.Value.Resolve()
		payload[attr.Key] = attr.Value.Any()
		return true
	})
	return json.Marshal(payload)
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
