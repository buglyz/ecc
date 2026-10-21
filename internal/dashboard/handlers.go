package dashboard

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/startup"
)

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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 页面内含本次启动生成的写接口令牌，禁止浏览器缓存旧页面导致令牌失效。
	w.Header().Set("Cache-Control", "no-store")
	if err := indexTemplate.Execute(w, indexData{Token: s.token}); err != nil {
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
	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONDecodeError(w, err)
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
	cfg, saveErr := s.saveAndApplyLocked(cfg)
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
	var req presetRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONDecodeError(w, err)
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

	_, saveErr := s.saveAndApplyLocked(cfg)
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
	var req startupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	var err error
	if req.Enabled {
		err = s.startup.Add(s.paths.StartupTarget, startup.Identifier)
	} else {
		err = s.startup.Remove(startup.Identifier)
	}
	if err != nil {
		if errors.Is(err, startup.ErrUnsafeStartupTarget) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	enabled := s.startup.IsEnabled(startup.Identifier)
	s.startupCached.Store(enabled)
	// elevated=false 表示目标不在 Program Files，计划任务以 LIMITED 运行，
	// 登录时由程序自行请求 UAC。
	s.writeJSON(w, map[string]any{"ok": true, "startup": enabled, "elevated": startup.RequiresElevation(s.paths.StartupTarget)})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "time": time.Now()})
}
