package dashboard

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

func startSecurityTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	p := paths.Paths{StateDir: dir, ConfigPath: filepath.Join(dir, "config.json")}
	cfg := config.Default()
	fan := controller.NewFanController(staticReader{}, okWriter{}, cfg.Curve, cfg.Strategy, 0, log.New(io.Discard, "", 0))
	server := New("127.0.0.1:0", p, &cfg, fan, log.New(io.Discard, "", 0))
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	return server
}

func doRequest(t *testing.T, method, target, body string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return resp
}

func TestWriteAPIRequiresToken(t *testing.T) {
	server := startSecurityTestServer(t)

	resp := doRequest(t, http.MethodPost, server.URL()+"/api/config", `{"theme":"dark"}`, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without token status=%d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doRequest(t, http.MethodPost, server.URL()+"/api/config", `{"theme":"dark"}`, func(r *http.Request) {
		r.Header.Set(tokenHeader, server.token)
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST with token status=%d, want 200", resp.StatusCode)
	}
}

func TestAPIRecjectsNonLoopbackHost(t *testing.T) {
	server := startSecurityTestServer(t)
	resp := doRequest(t, http.MethodGet, server.URL()+"/api/health", "", func(r *http.Request) {
		r.Host = "evil.example.com:8765"
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-loopback host status=%d, want 403", resp.StatusCode)
	}
}

func TestIndexSendsAntiFramingHeadersAndToken(t *testing.T) {
	server := startSecurityTestServer(t)
	resp := doRequest(t, http.MethodGet, server.URL()+"/", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status=%d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options=%q, want DENY", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(body), `name="ecc-token" content="`+server.token+`"`) {
		t.Fatal("index page does not contain the session token")
	}
}

func TestLoopbackHostsMatchesActualPort(t *testing.T) {
	server := startSecurityTestServer(t)
	u, err := url.Parse(server.URL())
	if err != nil {
		t.Fatalf("Parse URL: %v", err)
	}
	if _, ok := server.allowedHosts[u.Host]; !ok {
		t.Fatalf("allowedHosts=%v missing actual host %q", server.allowedHosts, u.Host)
	}
}
