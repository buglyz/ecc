package main

import (
	"testing"
	"time"
)

func TestHeartbeatHealthy(t *testing.T) {
	start := time.Unix(0, 0)

	if !heartbeatHealthy(time.Time{}, start, start.Add(heartbeatStartGrace-time.Second)) {
		t.Fatal("initial grace should keep heartbeat healthy")
	}
	if heartbeatHealthy(time.Time{}, start, start.Add(heartbeatStartGrace+time.Second)) {
		t.Fatal("heartbeat should be unhealthy after initial grace")
	}

	latest := start.Add(100 * time.Second)
	if !heartbeatHealthy(latest, start, latest.Add(heartbeatStaleAfter-time.Second)) {
		t.Fatal("fresh sample should keep heartbeat healthy")
	}
	if heartbeatHealthy(latest, start, latest.Add(heartbeatStaleAfter+time.Second)) {
		t.Fatal("stale sample should make heartbeat unhealthy")
	}
}
