package dashboard

import (
	"context"
	"encoding/json"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

	startupCached atomic.Bool
}

type stateResponse struct {
	Config       config.Config              `json:"config"`
	Latest       controller.Latest          `json:"latest"`
	History      []controller.HistorySample `json:"history"`
	Strategies   []controller.Strategy      `json:"strategies"`
	Presets      []controller.Preset        `json:"presets"`
	Startup      bool                       `json:"startup"`
	RuntimePaths paths.Paths                `json:"runtime_paths"`
}

type updateRequest struct {
	Curve         []controller.Point `json:"curve"`
	Strategy      string             `json:"strategy"`
	ManualEnabled *bool              `json:"manual_enabled"`
	ManualSpeed   *int               `json:"manual_speed"`
	Theme         string             `json:"theme"`
	TimeEntry     string             `json:"time_entry"`
}

type presetRequest struct {
	// Action：apply（切换，默认）/ save（写回当前曲线）/ add（新建自定义挡位）/
	// restore（内置挡位恢复出厂）/ delete（删除自定义挡位）。
	Action string `json:"action"`
	Key    string `json:"key"`
	Label  string `json:"label"`
}

type startupRequest struct {
	Enabled bool `json:"enabled"`
}

func New(addr string, p paths.Paths, cfg *config.Config, fan *controller.FanController, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{addr: addr, paths: p, cfg: cfg, controller: fan, logger: logger}
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
	s.startupCached.Store(startup.IsEnabled(startup.Identifier))
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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, nil); err != nil {
		s.logger.Printf("首页模板渲染失败: %v", err)
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	var history []controller.HistorySample
	if minutes := r.URL.Query().Get("minutes"); minutes != "" {
		if m, err := strconv.Atoi(minutes); err == nil && m > 0 {
			cutoff := time.Now().Add(-time.Duration(m) * time.Minute)
			history = s.controller.SnapshotSince(cutoff)
		}
	}
	if history == nil {
		history = s.controller.Snapshot()
	}
	s.writeJSON(w, stateResponse{
		Config:       s.configSnapshot(),
		Latest:       s.controller.Latest(),
		History:      history,
		Strategies:   controller.Strategies,
		Presets:      controller.Presets,
		Startup:      s.startupCached.Load(),
		RuntimePaths: s.paths,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !sameOrigin(r) {
		forbiddenOrigin(w)
		return
	}
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Strategy != "" && !controller.ValidStrategy(req.Strategy) {
		http.Error(w, "invalid strategy", http.StatusBadRequest)
		return
	}

	s.cfgMu.Lock()
	cfg := s.cfg.Clone()
	if len(req.Curve) > 0 {
		cfg.Curve = req.Curve
	}
	if req.Strategy != "" {
		cfg.Strategy = req.Strategy
	}
	if req.ManualSpeed != nil {
		cfg.ManualSpeed = clampInt(*req.ManualSpeed, 0, 100)
	}
	if req.ManualEnabled != nil {
		cfg.ManualEnabled = *req.ManualEnabled
	}
	if req.Theme == "light" || req.Theme == "dark" {
		cfg.Theme = req.Theme
	}
	if req.TimeEntry != "" {
		minutes, err := strconv.Atoi(req.TimeEntry)
		if err == nil {
			cfg.TimeEntry = strconv.Itoa(clampInt(minutes, 1, 480))
		}
	}
	// 显式保存模型：编辑只更新工作状态（curve/strategy），不自动写回激活挡位槽。
	// 否则槽永远等于工作状态，「已修改」标记和「保存」按钮便失去意义。
	// 挡位槽仅由 /api/preset 的 save/add/restore 动作变更。
	cfg = config.Normalize(cfg)
	saveErr := config.Save(s.paths, cfg)
	if saveErr == nil {
		*s.cfg = cfg.Clone()
		// 在锁内应用到控制器，确保「存盘顺序」与「应用顺序」一致：
		// 否则并发 POST 可能磁盘存 B、控制器却被 A 覆盖，永久不一致。
		s.applyControllerConfig(cfg)
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		http.Error(w, saveErr.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, map[string]any{"ok": true, "config": cfg})
}

func (s *Server) handlePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !sameOrigin(r) {
		forbiddenOrigin(w)
		return
	}
	var req presetRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := req.Action
	if action == "" {
		action = "apply"
	}

	s.cfgMu.Lock()
	cfg := s.cfg.Clone()
	// 显式保存模型：apply 不再自动把编辑写回旧槽，否则「已修改」标记与「保存」按钮便失去意义。
	// 工作状态（curve/strategy）由 /api/config 持久化；预设槽是命名快照，仅在 save/add/restore 时变更。
	var ok bool
	switch action {
	case "apply":
		ok = config.ApplyPreset(&cfg, req.Key)
	case "save":
		ok = config.SavePreset(&cfg, req.Key)
	case "add":
		ok = config.AddPreset(&cfg, newPresetKey(cfg), req.Label)
	case "restore":
		ok = config.RestorePreset(&cfg, req.Key)
	case "delete":
		ok = config.DeletePreset(&cfg, req.Key)
	default:
		s.cfgMu.Unlock()
		http.Error(w, "unknown preset action", http.StatusBadRequest)
		return
	}
	if !ok {
		s.cfgMu.Unlock()
		http.Error(w, "preset action rejected", http.StatusBadRequest)
		return
	}

	cfg = config.Normalize(cfg)
	saveErr := config.Save(s.paths, cfg)
	if saveErr == nil {
		*s.cfg = cfg.Clone()
		s.applyControllerConfig(cfg)
	}
	s.cfgMu.Unlock()
	if saveErr != nil {
		http.Error(w, saveErr.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, map[string]any{"ok": true, "config": cfg})
}

// newPresetKey 生成一个当前配置中未占用的自定义挡位 key（custom1、custom2…）。
func newPresetKey(cfg config.Config) string {
	for i := 1; ; i++ {
		key := "custom" + strconv.Itoa(i)
		if _, exists := cfg.Presets[key]; !exists {
			return key
		}
	}
}

func (s *Server) handleStartup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !sameOrigin(r) {
		forbiddenOrigin(w)
		return
	}
	var req startupRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var err error
	if req.Enabled {
		err = startup.Add(s.paths.StartupTarget, startup.Identifier)
	} else {
		err = startup.Remove(startup.Identifier)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	enabled := startup.IsEnabled(startup.Identifier)
	s.startupCached.Store(enabled)
	s.writeJSON(w, map[string]any{"ok": true, "startup": enabled})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "time": time.Now()})
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

func (s *Server) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(value); err != nil {
		s.logger.Printf("响应 JSON 编码失败: %v", err)
	}
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// sameOrigin 校验状态变更请求的来源，防御 CSRF / DNS-rebinding。
// 本服务以管理员权限运行且暴露可注册开机自启动高权限计划任务的写接口，
// 而浏览器「简单请求」（如 text/plain 的 fetch）不触发 CORS 预检，
// 任意恶意网页都能向 127.0.0.1 发 POST。真实 UI 走相对路径（同源），
// Origin 恒为本地回环；非浏览器客户端与单元测试不带 Origin，予以放行。
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// isLoopbackHost 报告主机名是否指向本机回环地址。
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func forbiddenOrigin(w http.ResponseWriter) {
	http.Error(w, "forbidden origin", http.StatusForbidden)
}

const maxRequestBody = 64 * 1024

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(v)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

func BindAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
