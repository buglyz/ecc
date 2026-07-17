package dashboard

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
)

type staticReader struct{}

func (staticReader) Read() controller.Temps { return controller.Temps{} }
func (staticReader) Close() error           { return nil }

type okWriter struct{}

func (okWriter) Write(context.Context, string, string) bool { return true }

func TestURLUsesActualEphemeralPort(t *testing.T) {
	cfg := config.Default()
	fan := controller.NewFanController(staticReader{}, okWriter{}, cfg.Curve, cfg.Strategy, 0, log.Default())
	server := New("127.0.0.1:0", structPaths(), &cfg, fan, log.Default())
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())
	if server.URL() == "http://127.0.0.1:0" {
		t.Fatal("URL returned the requested :0 address instead of the actual listener address")
	}
	resp, err := http.Get(server.URL() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
}

func TestConfigSnapshotConcurrentClone(t *testing.T) {
	cfg := config.Default()
	fan := controller.NewFanController(staticReader{}, okWriter{}, cfg.Curve, cfg.Strategy, 0, log.Default())
	server := New("127.0.0.1:0", structPaths(), &cfg, fan, log.Default())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(50 * time.Millisecond)
			for time.Now().Before(deadline) {
				_ = server.configSnapshot()
			}
		}()
	}
	wg.Wait()
}

func TestEmbeddedDashboardIsSelfContainedAndReportsActionErrors(t *testing.T) {
	for _, external := range []string{`src="http://`, `src="https://`, `href="http://`, `href="https://`} {
		if strings.Contains(indexHTML, external) {
			t.Fatalf("dashboard contains external asset reference %q", external)
		}
	}
	for _, marker := range []string{`id="toast"`, "function actionError", "refreshing=false"} {
		if !strings.Contains(indexHTML, marker) {
			t.Errorf("dashboard missing %q", marker)
		}
	}
}
