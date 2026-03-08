package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

// WorkerFunc is a function that performs work
type WorkerFunc func(ctx context.Context) error

// Worker represents a background worker
type Worker struct {
	Name     string
	Fn       WorkerFunc
	Interval time.Duration
}

// Supervisor manages worker goroutines with panic recovery and automatic restart
type Supervisor struct {
	workers      []Worker
	logger       *zap.Logger
	wg           sync.WaitGroup
	stopCh       chan struct{}
	restartCh    chan restartRequest
	maxRestarts  int
	restartWait  time.Duration
	startOnce    sync.Once
	stopOnce     sync.Once
	panicsTotal  metric.Int64Counter
	restartsTotal metric.Int64Counter
}

type restartRequest struct {
	worker Worker
	err    error
}

// SupervisorConfig holds supervisor configuration
type SupervisorConfig struct {
	MaxRestarts int           // Maximum restart attempts per worker (default: 5)
	RestartWait time.Duration // Wait time before restart (default: 5s)
}

// NewSupervisor creates a new Supervisor with the given configuration
func NewSupervisor(logger *zap.Logger, config SupervisorConfig) *Supervisor {
	if config.MaxRestarts == 0 {
		config.MaxRestarts = 5
	}
	if config.RestartWait == 0 {
		config.RestartWait = 5 * time.Second
	}

	meter := otel.Meter("padosme-be-common/worker")

	// Ignore errors — a no-op instrument is returned on failure, which is safe.
	panicsTotal, _ := meter.Int64Counter(
		"worker.panics.total",
		metric.WithDescription("Total worker panics recovered"),
		metric.WithUnit("{panic}"),
	)

	restartsTotal, _ := meter.Int64Counter(
		"worker.restarts.total",
		metric.WithDescription("Total worker restart attempts"),
		metric.WithUnit("{restart}"),
	)

	return &Supervisor{
		workers:       make([]Worker, 0),
		logger:        logger,
		stopCh:        make(chan struct{}),
		restartCh:     make(chan restartRequest, 10),
		maxRestarts:   config.MaxRestarts,
		restartWait:   config.RestartWait,
		panicsTotal:   panicsTotal,
		restartsTotal: restartsTotal,
	}
}

// AddWorker adds a worker to the supervisor
// Workers should be added before calling Start()
func (s *Supervisor) AddWorker(worker Worker) {
	s.workers = append(s.workers, worker)
}

// Start starts all workers with panic recovery and restart logic
func (s *Supervisor) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.logger.Info("starting supervisor", zap.Int("worker_count", len(s.workers)))

		// Start restart handler and track it in wait group.
		s.wg.Add(1)
		go s.handleRestarts(ctx)

		// Start all workers.
		for _, worker := range s.workers {
			s.wg.Add(1)
			go s.runWorker(ctx, worker)
		}
	})
}

// Stop stops all workers gracefully
func (s *Supervisor) Stop() {
	s.stopOnce.Do(func() {
		s.logger.Info("stopping supervisor")
		close(s.stopCh)
		s.wg.Wait()
		s.logger.Info("supervisor stopped")
	})
}

// runWorker runs a worker with panic recovery and interval-based execution
func (s *Supervisor) runWorker(ctx context.Context, worker Worker) {
	defer s.wg.Done()

	s.logger.Info("starting worker", zap.String("worker", worker.Name))

	defer func() {
		if r := recover(); r != nil {
			var panicErr error
			if e, ok := r.(error); ok {
				panicErr = e
			} else {
				panicErr = fmt.Errorf("panic: %v", r)
			}

			s.logger.Error("worker panic recovered",
				zap.String("worker", worker.Name),
				zap.Any("panic", r),
			)
			s.panicsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("worker", worker.Name)))
			s.requestRestart(worker, panicErr)
		}
	}()

	// For workers with interval, run in a loop
	if worker.Interval > 0 {
		s.runIntervalWorker(ctx, worker)
	} else {
		// For one-time or continuous workers, execute once. If execution
		// fails before supervisor shutdown/cancel, request restart.
		if err := s.safeExecute(ctx, worker); err != nil {
			if ctx.Err() != nil || err == context.Canceled || err == context.DeadlineExceeded {
				s.logger.Info("worker exited due to context cancellation",
					zap.String("worker", worker.Name),
					zap.Error(err),
				)
				return
			}

			s.logger.Error("worker execution error",
				zap.String("worker", worker.Name),
				zap.Error(err),
			)
			s.requestRestart(worker, err)
		}
	}
}

// runIntervalWorker runs a worker periodically at the specified interval
func (s *Supervisor) runIntervalWorker(ctx context.Context, worker Worker) {
	ticker := time.NewTicker(worker.Interval)
	defer ticker.Stop()

	// Run immediately first
	if err := s.safeExecute(ctx, worker); err != nil {
		s.logger.Error("worker execution error",
			zap.String("worker", worker.Name),
			zap.Error(err),
		)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("worker context cancelled",
				zap.String("worker", worker.Name),
			)
			return
		case <-s.stopCh:
			s.logger.Info("worker stopped",
				zap.String("worker", worker.Name),
			)
			return
		case <-ticker.C:
			if err := s.safeExecute(ctx, worker); err != nil {
				s.logger.Error("worker execution error",
					zap.String("worker", worker.Name),
					zap.Error(err),
				)
			}
		}
	}
}

// safeExecute executes the worker function with panic recovery
func (s *Supervisor) safeExecute(ctx context.Context, worker Worker) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("worker execution panic",
				zap.String("worker", worker.Name),
				zap.Any("panic", r),
			)
			s.panicsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("worker", worker.Name)))
			// Convert panic to error so callers can apply clear restart policy.
			if e, ok := r.(error); ok {
				err = e
				return
			}
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return worker.Fn(ctx)
}

func (s *Supervisor) requestRestart(worker Worker, err error) {
	req := restartRequest{worker: worker, err: err}

	select {
	case <-s.stopCh:
		return
	case s.restartCh <- req:
		return
	default:
		// Avoid deadlocking worker goroutines when restart channel is saturated.
		s.logger.Error("restart request dropped due to full channel",
			zap.String("worker", worker.Name),
			zap.Error(err),
		)
	}
}

// handleRestarts handles worker restart requests
func (s *Supervisor) handleRestarts(ctx context.Context) {
	defer s.wg.Done()

	restartCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case req := <-s.restartCh:
			worker := req.worker
			restartCounts[worker.Name]++
			count := restartCounts[worker.Name]

			if count > s.maxRestarts {
				s.logger.Error("worker permanently stopped",
					zap.String("worker", worker.Name),
					zap.Int("restart_count", count),
					zap.Int("max_restarts", s.maxRestarts),
					zap.Error(req.err),
				)
				s.restartsTotal.Add(ctx, 1, metric.WithAttributes(
					attribute.String("worker", worker.Name),
					attribute.String("outcome", "permanently_stopped"),
				))
				continue
			}

			// Wait before restart and respect shutdown/cancel while waiting.
			s.logger.Info("restarting worker",
				zap.String("worker", worker.Name),
				zap.Int("restart_count", count),
				zap.Duration("wait", s.restartWait),
				zap.Error(req.err),
			)
			s.restartsTotal.Add(ctx, 1, metric.WithAttributes(
				attribute.String("worker", worker.Name),
				attribute.String("outcome", "restarted"),
			))

			timer := time.NewTimer(s.restartWait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-s.stopCh:
				timer.Stop()
				return
			case <-timer.C:
			}

			s.wg.Add(1)
			go s.runWorker(ctx, worker)
		}
	}
}
