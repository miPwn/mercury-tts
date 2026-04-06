package tests

import (
	"net/http"
	"testing"
	"time"
)

const daemonBaseURL = "http://localhost:8091"

func requireLocalDaemon(t testing.TB) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(daemonBaseURL + "/health")
	if err != nil {
		t.Skipf("Local TTS daemon not available at %s: %v", daemonBaseURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("Local TTS daemon health check returned %d", resp.StatusCode)
	}
}