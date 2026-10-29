package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
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
	// token 是本次进程启动时生成的会话凭证，注入页面并用于校验所有写接口，
	// 防止本机低权限进程或 DNS rebinding 页面直接篡改风扇配置。
	token string
	// allowedHosts 只接受回环地址 + 实际监听端口，阻断 DNS rebinding。
	allowedHosts map[string]struct{}
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
	token, err := newSessionToken()
	if err != nil {
		return fmt.Errorf("生成会话令牌失败: %w", err)
	}
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.actualAddr = listener.Addr().String()
	s.token = token
	s.allowedHosts = loopbackHosts(s.actualAddr)
	handler := s.secure(rejectCrossOrigin(mux))
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
	s.controller.SetControlMode(cfg.ControlMode)
	s.controller.SetTargetConfig(cfg.TargetConfig())
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

const tokenHeader = "X-ECC-Token"

type indexData struct {
	Token string
}

func newSessionToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// loopbackHosts 根据实际监听地址生成允许的 Host 集合。服务只绑定 127.0.0.1，
// 因此除回环地址外的 Host（包括 DNS rebinding 使用的攻击者域名）一律拒绝。
func loopbackHosts(addr string) map[string]struct{} {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return map[string]struct{}{}
	}
	return map[string]struct{}{
		"127.0.0.1:" + port: {},
		"localhost:" + port: {},
	}
}

// secure 统一处理安全响应头、Host 白名单与写接口令牌校验。
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Content-Security-Policy", "frame-ancestors 'none'")
		header.Set("Referrer-Policy", "no-referrer")

		if _, ok := s.allowedHosts[strings.ToLower(r.Host)]; !ok {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if requiresSessionToken(r) && !s.tokenMatches(r.Header.Get(tokenHeader)) {
			http.Error(w, "invalid or missing token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requiresSessionToken 只保护会改变状态的 /api/* 请求；GET/HEAD 保留给状态查询
// 与重复实例探测使用。
func requiresSessionToken(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func (s *Server) tokenMatches(candidate string) bool {
	if s.token == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.token)) == 1
}

func BindAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
