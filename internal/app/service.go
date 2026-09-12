// Package app coordinates UI actions with persisted configuration and the fan controller.
package app

import (
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/startup"
)

type State struct {
	Config  config.Config
	Latest  controller.Latest
	Startup bool
}

type Update struct {
	Curve         []controller.Point
	Strategy      string
	ManualEnabled bool
	ManualSpeed   int
	Theme         string
}

type Service struct {
	paths      paths.Paths
	cfg        *config.Config
	controller *controller.FanController
	logger     *log.Logger
	startup    startupController

	mu             sync.RWMutex
	startupEnabled bool
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

func New(p paths.Paths, cfg *config.Config, fan *controller.FanController, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	service := &Service{
		paths:      p,
		cfg:        cfg,
		controller: fan,
		logger:     logger,
		startup:    systemStartupController{},
	}
	service.startupEnabled = service.startup.IsEnabled(startup.Identifier)
	return service
}

func (s *Service) State() State {
	s.mu.RLock()
	cfg := s.cfg.Clone()
	startupEnabled := s.startupEnabled
	s.mu.RUnlock()
	return State{Config: cfg, Latest: s.controller.Latest(), Startup: startupEnabled}
}

func (s *Service) Update(update Update) (config.Config, error) {
	if len(update.Curve) != controller.CurvePoints {
		return config.Config{}, fmt.Errorf("曲线必须包含 %d 个控制点", controller.CurvePoints)
	}
	if !controller.ValidStrategy(update.Strategy) {
		return config.Config{}, fmt.Errorf("无效的温度策略")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Clone()
	cfg.Curve = append([]controller.Point(nil), update.Curve...)
	cfg.Strategy = update.Strategy
	cfg.ManualEnabled = update.ManualEnabled
	cfg.ManualSpeed = update.ManualSpeed
	if update.Theme != "" {
		cfg.Theme = update.Theme
	}
	return s.saveAndApplyLocked(cfg)
}

func (s *Service) SetTheme(theme string) (config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Clone()
	cfg.Theme = theme
	return s.saveAndApplyLocked(cfg)
}

func (s *Service) ApplyPreset(key string) (config.Config, error) {
	return s.changePreset("apply", key, "")
}

func (s *Service) SavePreset(key string) (config.Config, error) {
	return s.changePreset("save", key, "")
}

func (s *Service) RestorePreset(key string) (config.Config, error) {
	return s.changePreset("restore", key, "")
}

func (s *Service) AddPreset(label string) (config.Config, error) {
	return s.changePreset("add", "", label)
}

func (s *Service) DeletePreset(key string) (config.Config, error) {
	return s.changePreset("delete", key, "")
}

func (s *Service) changePreset(action, key, label string) (config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Clone()

	var ok bool
	switch action {
	case "apply":
		ok = config.ApplyPreset(&cfg, key)
	case "save":
		ok = config.SavePreset(&cfg, key)
	case "restore":
		ok = config.RestorePreset(&cfg, key)
	case "add":
		ok = config.AddPreset(&cfg, newPresetKey(cfg), label)
	case "delete":
		ok = config.DeletePreset(&cfg, key)
	}
	if !ok {
		return config.Config{}, fmt.Errorf("预设操作未执行")
	}
	return s.saveAndApplyLocked(cfg)
}

func (s *Service) SetStartup(enabled bool) (bool, error) {
	var err error
	if enabled {
		err = s.startup.Add(s.paths.StartupTarget, startup.Identifier)
	} else {
		err = s.startup.Remove(startup.Identifier)
	}
	if err != nil {
		return false, err
	}

	actual := s.startup.IsEnabled(startup.Identifier)
	s.mu.Lock()
	s.startupEnabled = actual
	s.mu.Unlock()
	return actual, nil
}

func (s *Service) saveAndApplyLocked(cfg config.Config) (config.Config, error) {
	cfg = config.Normalize(cfg)
	if err := config.Save(s.paths, cfg); err != nil {
		return config.Config{}, err
	}
	*s.cfg = cfg.Clone()
	s.controller.SetCurve(cfg.Curve)
	s.controller.SetStrategy(cfg.Strategy)
	s.controller.SetManual(cfg.ManualSpeedPtr())
	s.logger.Printf("原生 UI 配置已保存: strategy=%s manual_enabled=%t manual_speed=%d", cfg.Strategy, cfg.ManualEnabled, cfg.ManualSpeed)
	return cfg, nil
}

func newPresetKey(cfg config.Config) string {
	for i := 1; ; i++ {
		key := "custom" + strconv.Itoa(i)
		if _, exists := cfg.Presets[key]; !exists {
			return key
		}
	}
}
