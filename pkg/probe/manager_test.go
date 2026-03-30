package probe

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestManager_HealthyProbe(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(ManagerConfig{
		DefaultInterval: 50 * time.Millisecond,
		DefaultTimeout:  1 * time.Second,
	}, logger)

	mgr.Register(ProbeConfig{
		Name:     "always-healthy",
		Critical: true,
		Check: func(ctx context.Context) error {
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	// Wait for at least one check to complete.
	time.Sleep(100 * time.Millisecond)

	if !mgr.IsHealthy() {
		t.Error("expected IsHealthy to be true")
	}

	s, ok := mgr.GetStatus("always-healthy")
	if !ok {
		t.Fatal("expected to find status for always-healthy")
	}
	if !s.Healthy {
		t.Error("expected probe to be healthy")
	}
	if s.LastCheck.IsZero() {
		t.Error("expected LastCheck to be set")
	}
}

func TestManager_UnhealthyProbe(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(ManagerConfig{
		DefaultInterval: 50 * time.Millisecond,
		DefaultTimeout:  1 * time.Second,
	}, logger)

	mgr.Register(ProbeConfig{
		Name:     "always-unhealthy",
		Critical: true,
		Check: func(ctx context.Context) error {
			return errors.New("connection refused")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)

	if mgr.IsHealthy() {
		t.Error("expected IsHealthy to be false with failing critical probe")
	}

	s, ok := mgr.GetStatus("always-unhealthy")
	if !ok {
		t.Fatal("expected to find status for always-unhealthy")
	}
	if s.Healthy {
		t.Error("expected probe to be unhealthy")
	}
	if s.LastError != "connection refused" {
		t.Errorf("expected LastError to be 'connection refused', got %q", s.LastError)
	}
}

func TestManager_Recovery(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(ManagerConfig{
		DefaultInterval: 50 * time.Millisecond,
		DefaultTimeout:  1 * time.Second,
	}, logger)

	var callCount atomic.Int32
	mgr.Register(ProbeConfig{
		Name:     "recoverable",
		Critical: true,
		Check: func(ctx context.Context) error {
			// Fail the first 3 checks, then succeed.
			if callCount.Add(1) <= 3 {
				return errors.New("not ready yet")
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	// Wait for initial unhealthy state.
	time.Sleep(80 * time.Millisecond)
	if mgr.IsHealthy() {
		t.Error("expected IsHealthy to be false initially")
	}

	// Wait for recovery.
	time.Sleep(200 * time.Millisecond)
	if !mgr.IsHealthy() {
		t.Error("expected IsHealthy to be true after recovery")
	}

	s, _ := mgr.GetStatus("recoverable")
	if !s.Healthy {
		t.Error("expected probe to be healthy after recovery")
	}
	if s.LastError != "" {
		t.Errorf("expected LastError to be cleared, got %q", s.LastError)
	}
}

func TestManager_NonCriticalFailure(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(ManagerConfig{
		DefaultInterval: 50 * time.Millisecond,
		DefaultTimeout:  1 * time.Second,
	}, logger)

	mgr.Register(ProbeConfig{
		Name:     "healthy-critical",
		Critical: true,
		Check: func(ctx context.Context) error {
			return nil
		},
	})

	mgr.Register(ProbeConfig{
		Name:     "failing-non-critical",
		Critical: false,
		Check: func(ctx context.Context) error {
			return errors.New("cache down")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	time.Sleep(100 * time.Millisecond)

	// IsHealthy should still be true because the failing probe is not critical.
	if !mgr.IsHealthy() {
		t.Error("expected IsHealthy to be true when only non-critical probes fail")
	}

	s, _ := mgr.GetStatus("failing-non-critical")
	if s.Healthy {
		t.Error("expected non-critical probe to report unhealthy")
	}

	all := mgr.AllStatuses()
	if len(all) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(all))
	}
}
