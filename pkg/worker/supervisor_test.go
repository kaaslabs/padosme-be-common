package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSupervisor_BasicExecution(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{
		MaxRestarts: 3,
		RestartWait: 100 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	executed := false
	worker := Worker{
		Name: "test-worker",
		Fn: func(ctx context.Context) error {
			executed = true
			return nil
		},
		Interval: 0, // One-time execution
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	supervisor.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	supervisor.Stop()

	if !executed {
		t.Error("worker was not executed")
	}
}

func TestSupervisor_IntervalWorker(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{
		MaxRestarts: 3,
		RestartWait: 100 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	execCount := 0
	worker := Worker{
		Name: "interval-worker",
		Fn: func(ctx context.Context) error {
			execCount++
			return nil
		},
		Interval: 100 * time.Millisecond,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	supervisor.Start(ctx)
	time.Sleep(450 * time.Millisecond)
	supervisor.Stop()

	// Should execute immediately + ~3-4 more times
	if execCount < 3 {
		t.Errorf("expected at least 3 executions, got %d", execCount)
	}
}

func TestSupervisor_PanicRecovery(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{
		MaxRestarts: 2,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	panicCount := 0
	worker := Worker{
		Name: "panic-worker",
		Fn: func(ctx context.Context) error {
			panicCount++
			if panicCount < 3 {
				panic("intentional panic for testing")
			}
			return nil
		},
		Interval: 0,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	supervisor.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	supervisor.Stop()

	// Should panic twice and restart
	if panicCount < 2 {
		t.Errorf("expected at least 2 panics, got %d", panicCount)
	}
}

func TestSupervisor_ErrorHandling(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{
		MaxRestarts: 1,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	execCount := 0
	worker := Worker{
		Name: "error-worker",
		Fn: func(ctx context.Context) error {
			execCount++
			if execCount == 1 {
				return errors.New("intentional error")
			}
			return nil
		},
		Interval: 100 * time.Millisecond,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	supervisor.Start(ctx)
	time.Sleep(400 * time.Millisecond)
	supervisor.Stop()

	// Worker should continue running despite errors
	if execCount < 2 {
		t.Errorf("expected at least 2 executions, got %d", execCount)
	}
}

func TestSupervisor_MaxRestarts(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{
		MaxRestarts: 2,
		RestartWait: 50 * time.Millisecond,
	}

	supervisor := NewSupervisor(logger, config)

	panicCount := 0
	worker := Worker{
		Name: "always-panic-worker",
		Fn: func(ctx context.Context) error {
			panicCount++
			panic("always panic")
		},
		Interval: 0,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	supervisor.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	supervisor.Stop()

	// Should panic max 3 times (initial + 2 restarts)
	if panicCount > 3 {
		t.Errorf("expected at most 3 panics, got %d", panicCount)
	}
}

func TestSupervisor_GracefulShutdown(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := SupervisorConfig{}
	supervisor := NewSupervisor(logger, config)

	shutdownCalled := false
	worker := Worker{
		Name: "long-running-worker",
		Fn: func(ctx context.Context) error {
			<-ctx.Done()
			shutdownCalled = true
			return ctx.Err()
		},
		Interval: 0,
	}

	supervisor.AddWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())

	supervisor.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	cancel()
	supervisor.Stop()

	if !shutdownCalled {
		t.Error("worker did not receive shutdown signal")
	}
}
