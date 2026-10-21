//go:build windows

package instance

import (
	"testing"
	"time"
)

func TestShutdownSignalClosesListener(t *testing.T) {
	name := `Local\ECC-Test-ShutdownSignal`
	listener, err := NewShutdownListener(name)
	if err != nil {
		t.Fatalf("NewShutdownListener: %v", err)
	}
	defer listener.Close()

	select {
	case <-listener.Done():
		t.Fatal("listener closed before any signal")
	case <-time.After(50 * time.Millisecond):
	}

	if err := SignalShutdown(name); err != nil {
		t.Fatalf("SignalShutdown: %v", err)
	}
	select {
	case <-listener.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not observe shutdown signal")
	}
}

func TestSignalShutdownWithoutInstanceReturnsNil(t *testing.T) {
	if err := SignalShutdown(`Local\ECC-Test-NoSuchInstance`); err != nil {
		t.Fatalf("SignalShutdown without instance: %v", err)
	}
}
