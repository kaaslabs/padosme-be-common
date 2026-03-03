package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func waitForSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func TestSupervisor_BasicExecution(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{
		MaxRestarts: 3,
		RestartWait: 100 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	executed := make(chan struct{})
	var once sync.Once
	worker := Worker{
		Name: "test-worker",
		Fn: func(ctx context.Context) error {
			once.Do(func() { close(executed) })
			return nil
		},
		Interval: 0, // One-time execution
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	defer supervisor.Stop()

	waitForSignal(t, executed, 2*time.Second, "worker was not executed")
}

func TestSupervisor_IntervalWorker(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{
		MaxRestarts: 3,
		RestartWait: 100 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	var execCount atomic.Int32
	done := make(chan struct{})
	var once sync.Once
	worker := Worker{
		Name: "interval-worker",
		Fn: func(ctx context.Context) error {
			if execCount.Add(1) >= 3 {
				once.Do(func() { close(done) })
			}
			return nil
		},
		Interval: 50 * time.Millisecond,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	defer supervisor.Stop()

	waitForSignal(t, done, 2*time.Second, "interval worker did not execute enough times")

	if count := execCount.Load(); count < 3 {
		t.Errorf("expected at least 3 executions, got %d", count)
	}
}

func TestSupervisor_PanicRecovery(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{
		MaxRestarts: 2,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	var panicCount atomic.Int32
	done := make(chan struct{})
	var once sync.Once
	worker := Worker{
		Name: "panic-worker",
		Fn: func(ctx context.Context) error {
			count := panicCount.Add(1)
			if count < 3 {
				panic("intentional panic for testing")
			}

			once.Do(func() { close(done) })
			return nil
		},
		Interval: 50 * time.Millisecond,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	defer supervisor.Stop()

	waitForSignal(t, done, 2*time.Second, "panic worker did not recover and continue")

	if count := panicCount.Load(); count < 3 {
		t.Errorf("expected at least 3 executions with initial panics, got %d", count)
	}
}

func TestSupervisor_ErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{
		MaxRestarts: 1,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	var execCount atomic.Int32
	done := make(chan struct{})
	var once sync.Once
	worker := Worker{
		Name: "error-worker",
		Fn: func(ctx context.Context) error {
			count := execCount.Add(1)
			if count == 1 {
				return errors.New("intentional error")
			}

			once.Do(func() { close(done) })
			return nil
		},
		Interval: 50 * time.Millisecond,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	defer supervisor.Stop()

	waitForSignal(t, done, 2*time.Second, "worker did not continue after initial error")

	// Worker should continue running despite errors
	if count := execCount.Load(); count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}
}

func TestSupervisor_MaxRestarts(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{
		MaxRestarts: 2,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	var panicCount atomic.Int32
	done := make(chan struct{})
	var once sync.Once
	worker := Worker{
		Name: "always-panic-worker",
		Fn: func(ctx context.Context) error {
			panicCount.Add(1)
			once.Do(func() { close(done) })
			panic("always panic")
		},
		Interval: 0,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	defer supervisor.Stop()

	waitForSignal(t, done, 1*time.Second, "worker did not execute")
	time.Sleep(250 * time.Millisecond)

	if count := panicCount.Load(); count != 3 {
		t.Errorf("expected exactly 3 executions (initial + 2 restarts), got %d", count)
	}
}

func TestSupervisor_GracefulShutdown(t *testing.T) {
	logger := zap.NewNop()

	config := SupervisorConfig{}
	supervisor := NewSupervisor(logger, config)

	started := make(chan struct{})
	shutdownCalled := make(chan struct{})
	var startOnce sync.Once
	var shutdownOnce sync.Once
	worker := Worker{
		Name: "long-running-worker",
		Fn: func(ctx context.Context) error {
			startOnce.Do(func() { close(started) })
			<-ctx.Done()
			shutdownOnce.Do(func() { close(shutdownCalled) })
			return ctx.Err()
		},
		Interval: 0,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	supervisor.Start(ctx)
	waitForSignal(t, started, 1*time.Second, "worker did not start")

	cancel()
	supervisor.Stop()

	waitForSignal(t, shutdownCalled, 1*time.Second, "worker did not receive shutdown signal")
}
