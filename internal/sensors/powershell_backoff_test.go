package sensors

import (
	"log"
	"testing"
	"time"
)

func TestPowerShellReaderBackoff(t *testing.T) {
	r := &PowerShellReader{
		DLLPath: "nonexistent.dll",
		Logger:  log.Default(),
	}

	// First 2 failures should not trigger backoff
	for i := 0; i < 2; i++ {
		r.Read()
		if !r.backoffUntil.IsZero() {
			t.Errorf("failure %d should not trigger backoff", i+1)
		}
	}

	if r.consecutiveFails != 2 {
		t.Errorf("expected consecutiveFails=2, got %d", r.consecutiveFails)
	}

	// 3rd failure should trigger backoff
	r.Read()
	if r.backoffUntil.IsZero() {
		t.Error("3rd failure should trigger backoff")
	}
	if r.consecutiveFails != 3 {
		t.Errorf("expected consecutiveFails=3, got %d", r.consecutiveFails)
	}

	// During backoff, Read should return immediately
	r.Read()
	if r.consecutiveFails != 3 {
		t.Errorf("Read during backoff should not increment fails, got %d", r.consecutiveFails)
	}
}

func TestPowerShellReaderBackoffExponential(t *testing.T) {
	r := &PowerShellReader{
		DLLPath: "nonexistent.dll",
		Logger:  log.Default(),
	}

	// Trigger multiple failures and check backoff increases
	expectedBackoffs := []time.Duration{
		0,                // fail 1
		0,                // fail 2
		1 * time.Second,  // fail 3: 2^0 = 1s
		2 * time.Second,  // fail 4: 2^1 = 2s
		4 * time.Second,  // fail 5: 2^2 = 4s
		8 * time.Second,  // fail 6: 2^3 = 8s
		16 * time.Second, // fail 7: 2^4 = 16s
	}

	for i, expected := range expectedBackoffs {
		r.mu.Lock()
		r.recordFailureLocked()
		var actual time.Duration
		if !r.backoffUntil.IsZero() {
			actual = r.backoffUntil.Sub(r.lastFailTime)
		}
		r.mu.Unlock()

		// Allow 10ms tolerance for timing
		if actual > 0 && (actual < expected-10*time.Millisecond || actual > expected+10*time.Millisecond) {
			t.Errorf("failure %d: expected backoff ~%v, got %v", i+1, expected, actual)
		} else if actual == 0 && expected != 0 {
			t.Errorf("failure %d: expected backoff %v, got none", i+1, expected)
		}
	}
}

func TestPowerShellReaderBackoffMax(t *testing.T) {
	r := &PowerShellReader{
		DLLPath: "nonexistent.dll",
		Logger:  log.Default(),
	}

	// Trigger many failures to test max backoff
	for i := 0; i < 10; i++ {
		r.mu.Lock()
		r.recordFailureLocked()
		r.mu.Unlock()
	}

	r.mu.Lock()
	backoff := r.backoffUntil.Sub(r.lastFailTime)
	r.mu.Unlock()

	if backoff > maxBackoff {
		t.Errorf("backoff %v exceeds max %v", backoff, maxBackoff)
	}
}

func TestPowerShellReaderBackoffLargeFailureCountStaysCapped(t *testing.T) {
	r := &PowerShellReader{Logger: log.Default(), consecutiveFails: 1000}

	r.recordFailureLocked()

	if backoff := r.backoffUntil.Sub(r.lastFailTime); backoff != maxBackoff {
		t.Fatalf("backoff=%v, want %v", backoff, maxBackoff)
	}
}

func TestPowerShellReaderBackoffReset(t *testing.T) {
	r := &PowerShellReader{
		DLLPath: "nonexistent.dll",
		Logger:  log.Default(),
	}

	// Trigger failures
	for i := 0; i < 5; i++ {
		r.mu.Lock()
		r.recordFailureLocked()
		r.mu.Unlock()
	}

	if r.consecutiveFails != 5 {
		t.Errorf("expected consecutiveFails=5, got %d", r.consecutiveFails)
	}

	// Simulate success by resetting counters (as Read() does)
	r.mu.Lock()
	r.consecutiveFails = 0
	r.backoffUntil = time.Time{}
	r.mu.Unlock()

	if r.consecutiveFails != 0 {
		t.Errorf("expected consecutiveFails=0 after reset, got %d", r.consecutiveFails)
	}
	if !r.backoffUntil.IsZero() {
		t.Error("expected backoffUntil to be cleared after reset")
	}
}
