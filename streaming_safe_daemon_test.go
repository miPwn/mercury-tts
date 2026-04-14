//go:build tts_experimental
// +build tts_experimental

package main

import (
	"os"
	"testing"
	"time"
)

func TestGetBindAddrPrefersExplicitBindAddr(t *testing.T) {
	t.Setenv("TTS_BIND_ADDR", "1.2.3.4:9999")
	t.Setenv("DAEMON_HOST", "0.0.0.0")
	t.Setenv("DAEMON_PORT", "8091")

	if got := getBindAddr(); got != "1.2.3.4:9999" {
		t.Fatalf("expected explicit bind addr, got %q", got)
	}
}

func TestGetBindAddrFallsBackToDaemonHostAndPort(t *testing.T) {
	os.Unsetenv("TTS_BIND_ADDR")
	t.Setenv("DAEMON_HOST", "127.0.0.1")
	t.Setenv("DAEMON_PORT", "18091")

	if got := getBindAddr(); got != "127.0.0.1:18091" {
		t.Fatalf("expected daemon host+port, got %q", got)
	}
}

func TestGetEnvDurationParsesDecimalSeconds(t *testing.T) {
	t.Setenv("TEST_DURATION_SECONDS", "0.30")

	got := getEnvDuration("TEST_DURATION_SECONDS", 2*time.Second)
	want := 300 * time.Millisecond
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
