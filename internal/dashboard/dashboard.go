package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/startup"
)

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
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.actualAddr = listener.Addr().String()
	s.startupCached.Store(startup.IsEnabled(startup.Identifier))
	s.httpServer = &http.Server{Addr: s.actualAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
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

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>风扇控制器</title>
<style>
:root{
  --bg:#eef2f8;--bg2:#f8fafd;--surface:#ffffff;--surface2:#f4f7fb;--border:#dde4ef;
  --text:#16203a;--muted:#6b7689;--accent:#2563eb;--accent-soft:#dbe7ff;
  --cool:#1f9d57;--warm:#e8920c;--hot:#e23b3b;--shadow:0 6px 24px rgba(20,40,80,.07);
}
body.dark{
  --bg:#0f141c;--bg2:#141b26;--surface:#1a2230;--surface2:#222c3c;--border:#2c374a;
  --text:#e6ebf2;--muted:#94a1b6;--accent:#5b9dff;--accent-soft:#23304a;
  --cool:#37c97a;--warm:#f2ad3c;--hot:#ff6464;--shadow:0 6px 28px rgba(0,0,0,.35);
}
*{box-sizing:border-box}
html,body{margin:0}
body{font-family:"Microsoft YaHei",Segoe UI,system-ui,sans-serif;color:var(--text);
  background:linear-gradient(180deg,var(--bg),var(--bg2));min-height:100vh;padding:22px;
  transition:background .3s,color .3s}
main{max-width:1200px;margin:0 auto}
h1{font-size:23px;margin:0;letter-spacing:.5px}
h3{margin:0 0 12px;font-size:14px;font-weight:600;color:var(--muted);text-transform:none}
.hero{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:18px;flex-wrap:wrap}
.brand{display:flex;align-items:center;gap:12px}
.dot{width:9px;height:9px;border-radius:50%;background:var(--cool);box-shadow:0 0 0 0 rgba(31,157,87,.5);
  transition:background .3s}
.dot.off{background:var(--hot)}
.dot.live{animation:pulse 2s infinite}
@keyframes pulse{0%{box-shadow:0 0 0 0 rgba(31,157,87,.45)}70%{box-shadow:0 0 0 7px rgba(31,157,87,0)}100%{box-shadow:0 0 0 0 rgba(31,157,87,0)}}
.sub{color:var(--muted);font-size:12.5px;margin:3px 0 0}
.card{background:var(--surface);border:1px solid var(--border);border-radius:16px;padding:17px;
  box-shadow:var(--shadow);transition:background .3s,border-color .3s}
.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:13px}
.stat{position:relative;overflow:hidden}
.stat .label{color:var(--muted);font-size:12.5px;display:flex;align-items:center;gap:6px}
.stat .value{font-size:36px;font-weight:750;margin-top:4px;line-height:1.05;font-variant-numeric:tabular-nums}
.stat .unit{font-size:16px;font-weight:600;color:var(--muted);margin-left:3px}
.stat .extra{font-size:11.5px;color:var(--muted);margin-top:5px;min-height:14px}
.cool{color:var(--cool)}.warm{color:var(--warm)}.hot{color:var(--hot)}
.gauge{height:7px;border-radius:6px;background:var(--surface2);margin-top:11px;overflow:hidden}
.gauge>i{display:block;height:100%;width:0;border-radius:6px;
  background:linear-gradient(90deg,var(--cool),var(--warm),var(--hot));transition:width .5s ease}
.layout{display:grid;grid-template-columns:1.55fr .9fr;gap:13px;margin-top:13px}
canvas#chart{width:100%;height:340px;display:block}
canvas#curveCanvas{width:100%;height:264px;cursor:crosshair;display:block;touch-action:none}
.controls{display:grid;gap:13px;align-content:start}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.chip-row{display:flex;gap:7px;flex-wrap:wrap}
button,select,input[type=number]{font:inherit;border-radius:10px;border:1px solid var(--border);
  padding:8px 12px;background:var(--surface);color:var(--text);transition:all .18s}
button{cursor:pointer;background:var(--accent);color:#fff;border-color:var(--accent);font-weight:600}
button:hover{filter:brightness(1.07)}
button:active{transform:translateY(1px)}
button.secondary{background:var(--surface);color:var(--text)}
button.secondary:hover{border-color:var(--accent);color:var(--accent)}
button.chip{background:var(--surface);color:var(--text);font-weight:500;padding:7px 13px}
button.chip.active{background:var(--accent);color:#fff;border-color:var(--accent)}
select{width:100%;cursor:pointer}
.modeline{color:var(--muted);font-size:12.5px;margin:12px 0 0;display:flex;justify-content:space-between;flex-wrap:wrap;gap:8px}
.modeline b{color:var(--text);font-weight:600}
.switch{display:flex;align-items:center;gap:9px;cursor:pointer;user-select:none;font-size:13.5px}
.switch input{position:absolute;opacity:0}
.track{width:40px;height:22px;border-radius:22px;background:var(--surface2);border:1px solid var(--border);
  position:relative;transition:background .2s}
.track::after{content:"";position:absolute;top:2px;left:2px;width:16px;height:16px;border-radius:50%;
  background:var(--muted);transition:transform .2s,background .2s}
.switch input:checked+.track{background:var(--accent);border-color:var(--accent)}
.switch input:checked+.track::after{transform:translateX(18px);background:#fff}
input[type=range]{-webkit-appearance:none;appearance:none;width:100%;height:7px;border-radius:6px;
  background:var(--surface2);margin:14px 0 6px;outline:none}
input[type=range]::-webkit-slider-thumb{-webkit-appearance:none;width:19px;height:19px;border-radius:50%;
  background:var(--accent);cursor:pointer;border:3px solid var(--surface);box-shadow:0 1px 4px rgba(0,0,0,.25)}
input[type=range]:disabled{opacity:.45}
.manualval{font-size:22px;font-weight:700;font-variant-numeric:tabular-nums}
.curve-head{display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap}
.muted{color:var(--muted);font-size:12.5px}
.preset-row{display:grid;grid-template-columns:repeat(3,1fr);gap:8px}
.preset-row button{padding:9px 0}
.preset-list{display:flex;flex-direction:column;gap:7px}
.preset-item{display:flex;align-items:center;gap:8px;padding:8px 10px;border:1px solid var(--border);
  border-radius:11px;background:var(--surface);transition:border-color .18s,background .18s}
.preset-item.active{border-color:var(--accent);background:var(--accent-soft)}
.preset-item .pname{flex:1;font-weight:600;font-size:13.5px;cursor:pointer;display:flex;align-items:center;gap:7px;min-width:0}
.preset-item .pname span.txt{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.preset-item .badge{font-size:10.5px;font-weight:700;color:#fff;background:var(--warm);
  padding:1px 7px;border-radius:20px;flex:none}
.preset-item .pacts{display:flex;gap:5px;flex:none}
.preset-item .pacts button{padding:5px 10px;font-size:12px;font-weight:600}
.preset-item .pacts button.ghost{background:transparent;color:var(--muted);border-color:var(--border)}
.preset-item .pacts button.ghost:hover{color:var(--accent);border-color:var(--accent)}
.preset-item .pacts button.danger{background:transparent;color:var(--hot);border-color:var(--border)}
.preset-item .pacts button.danger:hover{border-color:var(--hot)}
.preset-foot{display:flex;gap:7px;margin-top:9px}
.preset-foot input{flex:1;min-width:0}
@media(max-width:920px){.grid{grid-template-columns:repeat(2,1fr)}.layout{grid-template-columns:1fr}.hero{align-items:flex-start}}
@media(max-width:520px){.grid{grid-template-columns:1fr}}
</style>
</head>
<body>
<main>
  <div class="hero">
    <div class="brand">
      <span class="dot live" id="connDot" title="连接状态"></span>
      <div><h1>风扇控制器</h1><p class="sub" id="connText">实时温度监控 · 自动曲线调速 · 手动转速锁定</p></div>
    </div>
    <div class="row">
      <button class="secondary" id="theme">切换主题</button>
      <button class="secondary" id="startup">开机自启动</button>
    </div>
  </div>
  <section class="grid">
    <div class="card stat"><div class="label">CPU 温度</div><div class="value" id="cpu">--<span class="unit">°C</span></div><div class="extra" id="cpuExtra"></div></div>
    <div class="card stat"><div class="label">GPU 温度</div><div class="value" id="gpu">--<span class="unit">°C</span></div><div class="extra" id="gpuExtra"></div></div>
    <div class="card stat"><div class="label">目标温度</div><div class="value" id="target">--<span class="unit">°C</span></div><div class="extra" id="targetExtra"></div></div>
    <div class="card stat"><div class="label">风扇输出</div><div class="value" id="speed">--<span class="unit">%</span></div><div class="gauge"><i id="speedBar"></i></div></div>
  </section>
  <section class="layout">
    <div class="card">
      <canvas id="chart" width="860" height="340"></canvas>
      <p class="modeline"><span>模式：<b id="mode">--</b></span><span id="writeAgo"></span></p>
    </div>
    <div class="controls">
      <div class="card"><div class="curve-head" style="margin-bottom:10px"><h3 style="margin:0">预设挡位</h3><button class="chip" id="addPreset">+ 新建挡位</button></div><div class="preset-list" id="presetList"></div></div>
      <div class="card"><h3>温度策略</h3><select id="strategy"></select></div>
      <div class="card"><h3>手动模式</h3>
        <label class="switch"><input type="checkbox" id="manualEnabled"><span class="track"></span><span>锁定转速</span></label>
        <input type="range" id="manualSpeed" min="0" max="100">
        <div><span class="manualval" id="manualValue">--</span></div>
      </div>
      <div class="card"><h3>历史范围</h3>
        <div class="chip-row" id="rangeChips">
          <button class="chip" data-min="1">1 分</button>
          <button class="chip" data-min="5">5 分</button>
          <button class="chip" data-min="15">15 分</button>
          <button class="chip" data-min="30">30 分</button>
          <button class="chip" data-min="60">60 分</button>
        </div>
        <div class="row" style="margin-top:9px"><input id="minutes" type="number" min="1" max="480" style="width:90px"><span class="muted">分钟（1-480）</span></div>
      </div>
    </div>
  </section>
  <section class="card" style="margin-top:13px">
    <div class="curve-head"><h3 style="margin:0">风扇曲线（拖动控制点）</h3><span class="muted" id="curveValues"></span></div>
    <canvas id="curveCanvas" width="1100" height="264" style="margin-top:10px"></canvas>
  </section>
</main>
<script>
let state=null,dirty=false,saving=false;const $=id=>document.getElementById(id);function fmt(v){return v==null?'--':Math.round(v)}
function tempClass(v){if(v==null)return 'muted';if(v<60)return 'cool';if(v<80)return 'warm';return 'hot'}
function setTemp(id,extraId,v,stats){const el=$(id);el.className='value '+tempClass(v);el.innerHTML=fmt(v)+'<span class="unit">°C</span>';if(extraId&&stats){$(extraId).textContent=stats}}
function statsFor(history,key){let mn=Infinity,mx=-Infinity,sum=0,n=0;history.forEach(p=>{const v=p[key];if(v==null)return;mn=Math.min(mn,v);mx=Math.max(mx,v);sum+=v;n++});if(!n)return '';return '窗口 均'+Math.round(sum/n)+'° 峰'+Math.round(mx)+'°'}
function relTime(iso){if(!iso)return '';const d=Date.parse(iso);if(isNaN(d))return '';const s=Math.max(0,Math.round((Date.now()-d)/1000));if(s<60)return s+' 秒前';if(s<3600)return Math.round(s/60)+' 分前';return Math.round(s/3600)+' 时前'}
async function api(path,body){const res=await fetch(path,{method:body?'POST':'GET',headers:{'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined});if(!res.ok)throw new Error(await res.text());return res.json()}
function draw(history){const c=$('chart'),x=c.getContext('2d'),w=c.width,h=c.height;const dark=document.body.classList.contains('dark');const fg=dark?'#94a1b6':'#6b7689';const grid=dark?'#2c374a':'#dde4ef';const padL=44,padR=12,padT=24,padB=30;x.clearRect(0,0,w,h);x.font='12px "Microsoft YaHei",Segoe UI,sans-serif';x.fillStyle=fg;x.strokeStyle=grid;x.lineWidth=1;for(let i=0;i<=5;i++){let v=i*20,y=h-padB-v/100*(h-padT-padB);x.beginPath();x.moveTo(padL,y);x.lineTo(w-padR,y);x.stroke();x.fillStyle=fg;x.textAlign='right';x.textBaseline='middle';x.fillText(v,padL-6,y)}x.textAlign='left';x.textBaseline='top';x.fillStyle=fg;x.fillText('°C / %',6,4);const cutoff=Date.now()-Number($('minutes').value||5)*60000;history=history.filter(p=>Date.parse(p.time)>=cutoff);if(!history.length){x.fillStyle=fg;x.textAlign='center';x.fillText('等待温度数据...',w/2,h/2);return}const t0=Date.parse(history[0].time),t1=Date.parse(history[history.length-1].time)||t0+1;function line(key,color,fill){x.strokeStyle=color;x.lineWidth=2;x.beginPath();let started=false,first=null,last=null;history.forEach(p=>{let v=p[key];if(v==null)return;let px=padL+(Date.parse(p.time)-t0)/Math.max(1,t1-t0)*(w-padL-padR),py=h-padB-v/100*(h-padT-padB);if(!started){x.moveTo(px,py);started=true;first=px}else{x.lineTo(px,py)}last=px});x.stroke();if(fill&&started){x.lineTo(last,h-padB);x.lineTo(first,h-padB);x.closePath();x.globalAlpha=.10;x.fillStyle=color;x.fill();x.globalAlpha=1}}line('speed','#2b8a3e',true);line('cpu','#e23b3b');line('gpu','#1864ab');line('target_temp','#e8920c');const ticks=4;for(let i=0;i<=ticks;i++){let ts=t0+(t1-t0)*i/ticks,px=padL+i*(w-padL-padR)/ticks;const d=new Date(ts);const label=String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')+':'+String(d.getSeconds()).padStart(2,'0');x.fillStyle=fg;x.textAlign=i===0?'left':(i===ticks?'right':'center');x.textBaseline='top';x.fillText(label,px,h-padB+6)}const legend=[['CPU','#e23b3b'],['GPU','#1864ab'],['目标','#e8920c'],['风扇','#2b8a3e']];let lx=padL+8;legend.forEach(([label,color])=>{x.fillStyle=color;x.fillRect(lx,6,12,10);x.fillStyle=fg;x.textAlign='left';x.textBaseline='top';x.fillText(label,lx+16,6);lx+=16+x.measureText(label).width+14})}
function drawCurve(){const c=$('curveCanvas');if(!c||!state)return;const x=c.getContext('2d'),w=c.width,h=c.height;const dark=document.body.classList.contains('dark');const fg=dark?'#94a1b6':'#6b7689';const grid=dark?'#2c374a':'#dde4ef';const accent=dark?'#5b9dff':'#2563eb';const padL=46,padR=14,padT=18,padB=30;x.clearRect(0,0,w,h);x.font='12px "Microsoft YaHei",Segoe UI,sans-serif';x.strokeStyle=grid;x.lineWidth=1;x.fillStyle=fg;for(let s=0;s<=10;s+=2){const v=s*10,y=h-padB-v/100*(h-padT-padB);x.beginPath();x.moveTo(padL,y);x.lineTo(w-padR,y);x.stroke();x.textAlign='right';x.textBaseline='middle';x.fillText(v+'%',padL-6,y)}for(let t=30;t<=100;t+=10){const px=padL+(t-30)/70*(w-padL-padR);x.beginPath();x.moveTo(px,padT);x.lineTo(px,h-padB);x.stroke();x.textAlign='center';x.textBaseline='top';x.fillText(t+'°',px,h-padB+6)}const cur=state.latest&&state.latest.target_temp;if(cur!=null){const px=padL+(Math.max(30,Math.min(100,cur))-30)/70*(w-padL-padR);x.strokeStyle=dark?'#f2ad3c':'#e8920c';x.setLineDash([4,4]);x.beginPath();x.moveTo(px,padT);x.lineTo(px,h-padB);x.stroke();x.setLineDash([])}const curve=state.config.curve;const pts=curve.map(p=>[padL+(p[0]-30)/70*(w-padL-padR),h-padB-p[1]/100*(h-padT-padB)]);x.strokeStyle=accent;x.lineWidth=2.5;x.beginPath();pts.forEach(([px,py],i)=>{if(i===0)x.moveTo(px,py);else x.lineTo(px,py)});x.stroke();pts.forEach(([px,py],i)=>{x.fillStyle=accent;x.beginPath();x.arc(px,py,7,0,Math.PI*2);x.fill();x.strokeStyle=dark?'#1a2230':'#fff';x.lineWidth=2;x.stroke();x.fillStyle=fg;x.font='11px "Microsoft YaHei",Segoe UI,sans-serif';x.textAlign='center';x.textBaseline='bottom';x.fillText(curve[i][0]+'°/'+curve[i][1]+'%',px,py-12)});const vals=$('curveValues');if(vals)vals.textContent=curve.map(p=>'('+p[0]+'°, '+p[1]+'%)').join(' · ')}
function curveCoords(ev){const c=$('curveCanvas');const rect=c.getBoundingClientRect();const sx=c.width/rect.width,sy=c.height/rect.height;return{mx:(ev.clientX-rect.left)*sx,my:(ev.clientY-rect.top)*sy,w:c.width,h:c.height,padL:46,padR:14,padT:18,padB:30}}
let curveDrag=null;
function onCurveDown(ev){if(!state)return;const co=curveCoords(ev);const curve=state.config.curve;for(let i=0;i<curve.length;i++){const px=co.padL+(curve[i][0]-30)/70*(co.w-co.padL-co.padR),py=co.h-co.padB-curve[i][1]/100*(co.h-co.padT-co.padB);if(Math.hypot(co.mx-px,co.my-py)<=12){curveDrag={i};dirty=true;ev.preventDefault();break}}}
function onCurveMove(ev){if(!curveDrag||!state)return;const co=curveCoords(ev);let t=30+(co.mx-co.padL)/(co.w-co.padL-co.padR)*70,s=(co.h-co.padB-co.my)/(co.h-co.padT-co.padB)*100;t=Math.max(30,Math.min(100,t));s=Math.max(0,Math.min(100,s));const curve=state.config.curve,i=curveDrag.i;if(i>0)t=Math.max(t,curve[i-1][0]+0.5);if(i<curve.length-1)t=Math.min(t,curve[i+1][0]-0.5);curve[i]=[Math.round(t*10)/10,Math.round(s*10)/10];drawCurve()}
function onCurveUp(){if(curveDrag){curveDrag=null;save().catch(err=>{saving=false;console.error(err)})}}
function render(s){const wasDirty=dirty||saving||curveDrag;
  // dirty/拖动/保存期间保留本地未保存的工作状态，避免每秒轮询用服务器旧值覆盖正在编辑的曲线/策略。
  if(wasDirty&&state&&state.config){s.config.curve=state.config.curve;s.config.strategy=state.config.strategy}
  state=s;document.body.classList.toggle('dark',s.config.theme==='dark');
  setTemp('cpu','cpuExtra',s.latest.cpu,statsFor(s.history,'cpu'));
  setTemp('gpu','gpuExtra',s.latest.gpu,statsFor(s.history,'gpu'));
  setTemp('target','targetExtra',s.latest.target_temp,statsFor(s.history,'target_temp'));
  const sp=s.latest.speed;$('speed').innerHTML=fmt(sp)+'<span class="unit">%</span>';$('speedBar').style.width=(sp==null?0:Math.max(0,Math.min(100,sp)))+'%';
  $('mode').textContent=s.latest.mode||'--';
  const ago=relTime(s.latest.last_ec_write);$('writeAgo').textContent=ago?('上次写入 '+ago):'';
  $('startup').textContent=s.startup?'移除开机自启动':'添加开机自启动';
  renderPresets(s);
  if(!wasDirty){$('manualEnabled').checked=!!s.config.manual_enabled;$('manualSpeed').value=s.config.manual_speed;$('manualSpeed').disabled=!s.config.manual_enabled;$('manualValue').textContent=s.config.manual_speed+' %';$('minutes').value=s.config.time_entry;$('strategy').innerHTML=s.strategies.map(v=>'<option value="'+v.key+'">'+v.label+'</option>').join('');$('strategy').value=s.config.strategy;syncRangeChips()}
  draw(s.history);drawCurve()}
function syncRangeChips(){const m=$('minutes').value;document.querySelectorAll('#rangeChips [data-min]').forEach(b=>b.classList.toggle('active',b.dataset.min===String(m)))}
function curveEq(a,b){if(!a||!b||a.length!==b.length)return false;for(let i=0;i<a.length;i++){if(a[i][0]!==b[i][0]||a[i][1]!==b[i][1])return false}return true}
function presetModified(s){const slot=s.config.presets&&s.config.presets[s.config.active_preset];if(!slot)return false;return !curveEq(slot.curve,s.config.curve)||slot.strategy!==s.config.strategy}
const BUILTIN={silent:1,balanced:1,performance:1};
function presetAct(action,key){const body=action==='add'?{action,label:key}:{action,key};return api('/api/preset',body).then(()=>{dirty=false;return refresh()}).catch(e=>console.error(e))}
let presetSig='';
function renderPresets(s){const list=$('presetList');if(!list)return;const ps=s.config.presets||{};const active=s.config.active_preset;const modified=presetModified(s);
  const order=Object.keys(ps).sort((a,b)=>{const ba=BUILTIN[a]?0:1,bb=BUILTIN[b]?0:1;return ba!==bb?ba-bb:a<b?-1:1});
  // 仅当结构（挡位集合/名称/激活项/修改态）变化时才重建 DOM，避免每秒轮询重绘并丢失点击。
  const sig=order.map(k=>k+':'+(ps[k].label||k)).join('|')+'#'+active+'#'+(modified?1:0);
  if(sig===presetSig)return;presetSig=sig;
  let html='';
  order.forEach(key=>{const p=ps[key];const isActive=key===active;const builtin=!!BUILTIN[key];const label=(p.label||key);const k=esc(key);
    html+='<div class="preset-item'+(isActive?' active':'')+'" data-key="'+k+'">';
    html+='<div class="pname" data-act="apply" data-key="'+k+'"><span class="txt">'+esc(label)+'</span>';
    if(isActive&&modified)html+='<span class="badge">已修改</span>';
    html+='</div><div class="pacts">';
    if(isActive&&modified)html+='<button data-act="save" data-key="'+k+'">保存</button>';
    if(builtin)html+='<button class="ghost" data-act="restore" data-key="'+k+'">恢复默认</button>';
    else html+='<button class="danger" data-act="delete" data-key="'+k+'">删除</button>';
    html+='</div></div>'});
  list.innerHTML=html;
  list.querySelectorAll('[data-act]').forEach(el=>el.onclick=()=>{const act=el.dataset.act,key=el.dataset.key;const cur=(state.config.presets&&state.config.presets[key]);const nm=(cur&&cur.label)||key;
    if(act==='delete'){if(!confirm('确定删除挡位「'+nm+'」？'))return}
    if(act==='restore'){if(!confirm('将「'+nm+'」恢复为出厂参数？'))return}
    presetAct(act,key)})}
function esc(s){return String(s).replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]))}
function setConn(ok){const d=$('connDot');d.classList.toggle('off',!ok);d.classList.toggle('live',ok);$('connText').textContent=ok?'实时温度监控 · 自动曲线调速 · 手动转速锁定':'连接已断开，正在重试…'}
async function refresh(){try{const m=$('minutes').value||5;render(await api('/api/state?minutes='+m));setConn(true);document.title='风扇控制器'}catch(e){setConn(false);document.title='风扇控制器 [离线]';console.error(e)}}
async function save(extra={}){saving=true;try{const curve=state.config.curve.map(p=>[p[0],p[1]]);await api('/api/config',Object.assign({curve,strategy:$('strategy').value,manual_enabled:$('manualEnabled').checked,manual_speed:Number($('manualSpeed').value),theme:document.body.classList.contains('dark')?'dark':'light',time_entry:$('minutes').value},extra));dirty=false;await refresh()}finally{saving=false}}
document.addEventListener('input',e=>{if(['strategy','manualEnabled','manualSpeed','minutes'].includes(e.target.id))dirty=true});
document.addEventListener('change',e=>{if(['strategy','manualEnabled','manualSpeed','minutes'].includes(e.target.id)){if(e.target.id==='minutes')syncRangeChips();save().catch(err=>{saving=false;console.error(err)})}});
$('manualSpeed').addEventListener('input',()=>{$('manualValue').textContent=$('manualSpeed').value+' %'});
$('manualEnabled').addEventListener('change',()=>{$('manualSpeed').disabled=!$('manualEnabled').checked});
$('theme').onclick=()=>{document.body.classList.toggle('dark');dirty=true;save({theme:document.body.classList.contains('dark')?'dark':'light'}).catch(err=>{saving=false;console.error(err)})};
$('startup').onclick=async()=>{try{await api('/api/startup',{enabled:!state.startup});refresh()}catch(e){console.error(e)}};
$('addPreset').onclick=()=>{const name=(prompt('新挡位名称：','自定义挡位')||'').trim();if(!name)return;presetAct('add',name)};
document.querySelectorAll('#rangeChips [data-min]').forEach(b=>b.onclick=()=>{$('minutes').value=b.dataset.min;dirty=true;syncRangeChips();save().catch(err=>{saving=false;console.error(err)})});
const cc=$('curveCanvas');if(cc){cc.addEventListener('pointerdown',e=>{cc.setPointerCapture(e.pointerId);onCurveDown(e)});cc.addEventListener('pointermove',onCurveMove);cc.addEventListener('pointerup',onCurveUp);cc.addEventListener('pointercancel',onCurveUp)}
refresh();setInterval(refresh,1000);
</script>
</body>
</html>`))

func BindAddress(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
