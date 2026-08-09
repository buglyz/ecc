package dashboard

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/startup"
)

//go:embed web/index.html
var indexHTML string

type Server struct {
	addr       string
	actualAddr string
	paths      paths.Paths
	cfg        *config.Config
	cfgMu      sync.RWMutex
	controller *controller.FanController
	logger     *log.Logger
	httpServer *http.Server
	startup    startupController

	startupCached atomic.Bool
}

type startupController interface {
	IsEnabled(identifier string) bool
	Add(targetPath, identifier string) error
	Remove(identifier string) error
}

type systemStartupController struct{}

func (systemStartupController) IsEnabled(identifier string) bool {
	return startup.IsEnabled(identifier)
}

func (systemStartupController) Add(targetPath, identifier string) error {
	return startup.Add(targetPath, identifier)
}

func (systemStartupController) Remove(identifier string) error {
	return startup.Remove(identifier)
}

func New(addr string, p paths.Paths, cfg *config.Config, fan *controller.FanController, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{addr: addr, paths: p, cfg: cfg, controller: fan, logger: logger, startup: systemStartupController{}}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/preset", s.handlePreset)
	mux.HandleFunc("/api/startup", s.handleStartup)
	mux.HandleFunc("/api/health", s.handleHealth)
	handler := rejectCrossOrigin(mux)
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.actualAddr = listener.Addr().String()
	s.startupCached.Store(s.startup.IsEnabled(startup.Identifier))
	s.httpServer = &http.Server{Addr: s.actualAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	s.logger.Printf("Web 控制台已启动: http://%s", s.actualAddr)
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("Web 控制台异常退出: %v", err)
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) URL() string {
	if s.actualAddr == "" {
		return "http://" + s.addr
	}
	return "http://" + s.actualAddr
}

// rejectCrossOrigin wraps handler to block cross-origin requests to /api/*.
// A request with an Origin header whose host does not match the request's
// Host header is rejected with 403. Requests without Origin (same-origin
// navigation, curl, etc.) are allowed through.
func rejectCrossOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) configSnapshot() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Clone()
}

func (s *Server) applyControllerConfig(cfg config.Config) {
	s.controller.SetCurve(cfg.Curve)
	s.controller.SetStrategy(cfg.Strategy)
	s.controller.SetManual(cfg.ManualSpeedPtr())
}

// saveAndApplyLocked 把配置原子地「存盘 + 写入内存 + 应用到控制器」，并返回
// 归一化后的配置（响应回显用）。必须在持有 s.cfgMu 时调用。成功后先更新
// *s.cfg 再应用控制器，保证「存盘顺序」与「应用顺序」一致：并发 POST 不会出现
// 磁盘存 B、控制器却被 A 覆盖的永久不一致。返回错误时 *s.cfg 与控制器保持原样。
func (s *Server) saveAndApplyLocked(cfg config.Config) (config.Config, error) {
	cfg = config.Normalize(cfg)
	if saveErr := config.Save(s.paths, cfg); saveErr != nil {
		return cfg, saveErr
	}
	*s.cfg = cfg.Clone()
	s.applyControllerConfig(cfg)
	return cfg, nil
}

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

func BindAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}