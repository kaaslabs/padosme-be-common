// Package probe provides a background health-probe manager that continuously
// checks infrastructure dependencies (databases, caches, message brokers) and
// exposes their aggregated status. Unlike the health.Handler which runs probes
// on every HTTP request, the Manager runs probes on fixed intervals in the
// background so that status queries are always instant.
package probe

import (
	"context"
	"sync"
	"time"

	"github.com/kaaslabs/padosme-be-common/v4/pkg/health"
	"go.uber.org/zap"
)

// Status represents the current health state of a probe.
type Status struct {
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	LastCheck time.Time `json:"last_check"`
	LastError string    `json:"last_error,omitempty"`
	Critical  bool      `json:"critical"`
}

// CheckFunc is a function that checks the health of a dependency.
type CheckFunc func(ctx context.Context) error

// ProbeConfig defines a single probe.
type ProbeConfig struct {
	Name     string
	Check    CheckFunc
	Critical bool          // If true, failure makes the service unhealthy
	Interval time.Duration // How often to check (default: 5s)
	Timeout  time.Duration // Per-check timeout (default: 3s)
}

// ManagerConfig configures the probe manager.
type ManagerConfig struct {
	DefaultInterval time.Duration // Default check interval (default: 5s)
	DefaultTimeout  time.Duration // Default per-check timeout (default: 3s)
}

// Manager runs background health probes and exposes their status.
type Manager struct {
	probes []ProbeConfig
	status map[string]*Status // protected by mu
	mu     sync.RWMutex
	logger *zap.Logger
	cfg    ManagerConfig
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a probe Manager.
func NewManager(cfg ManagerConfig, logger *zap.Logger) *Manager {
	if cfg.DefaultInterval == 0 {
		cfg.DefaultInterval = 5 * time.Second
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 3 * time.Second
	}
	return &Manager{
		status: make(map[string]*Status),
		logger: logger,
		cfg:    cfg,
	}
}

// Register adds a probe. Must be called before Start.
func (m *Manager) Register(cfg ProbeConfig) {
	if cfg.Interval == 0 {
		cfg.Interval = m.cfg.DefaultInterval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = m.cfg.DefaultTimeout
	}
	m.probes = append(m.probes, cfg)
	m.status[cfg.Name] = &Status{
		Name:     cfg.Name,
		Critical: cfg.Critical,
	}
}

// Start begins all probe goroutines. Each probe runs in its own goroutine
// with its own ticker. Call Stop() to shut them all down.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	for _, p := range m.probes {
		m.wg.Add(1)
		go m.runProbe(ctx, p)
	}
}

// Stop stops all probe goroutines.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// IsHealthy returns true if all critical probes are passing.
func (m *Manager) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.status {
		if s.Critical && !s.Healthy {
			return false
		}
	}
	return true
}

// GetStatus returns the current status of a specific probe.
func (m *Manager) GetStatus(name string) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.status[name]
	if !ok {
		return Status{}, false
	}
	return *s, true
}

// AllStatuses returns a snapshot of all probe statuses.
func (m *Manager) AllStatuses() map[string]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]Status, len(m.status))
	for k, v := range m.status {
		result[k] = *v
	}
	return result
}

// RegisterWithHealthHandler wires all probes into a health.Handler.
// Each probe's latest status is exposed as a health probe function.
func (m *Manager) RegisterWithHealthHandler(h *health.Handler) {
	for _, p := range m.probes {
		name := p.Name
		critical := p.Critical
		h.Register(name, func(ctx context.Context) error {
			m.mu.RLock()
			s, ok := m.status[name]
			m.mu.RUnlock()
			if !ok {
				return nil
			}
			if !s.Healthy && s.LastError != "" {
				return errUnhealthy(s.LastError)
			}
			return nil
		}, critical)
	}
}

// errUnhealthy is a simple error type for unhealthy probes.
type errUnhealthy string

func (e errUnhealthy) Error() string { return string(e) }

// runProbe runs a single probe in its own goroutine on the configured interval.
func (m *Manager) runProbe(ctx context.Context, p ProbeConfig) {
	defer m.wg.Done()

	// Run immediately on start.
	m.executeProbe(ctx, p)

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.executeProbe(ctx, p)
		}
	}
}

// executeProbe runs a single probe check and updates the status map.
func (m *Manager) executeProbe(ctx context.Context, p ProbeConfig) {
	checkCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	err := p.Check(checkCtx)

	m.mu.Lock()
	s := m.status[p.Name]
	wasHealthy := s.Healthy
	s.LastCheck = time.Now()

	if err != nil {
		s.Healthy = false
		s.LastError = err.Error()
	} else {
		s.Healthy = true
		s.LastError = ""
	}

	nowHealthy := s.Healthy
	m.mu.Unlock()

	// Log state transitions.
	if wasHealthy && !nowHealthy {
		m.logger.Warn("probe became unhealthy",
			zap.String("probe", p.Name),
			zap.Bool("critical", p.Critical),
			zap.Error(err),
		)
	} else if !wasHealthy && nowHealthy {
		m.logger.Info("probe recovered",
			zap.String("probe", p.Name),
			zap.Bool("critical", p.Critical),
		)
	}
}
