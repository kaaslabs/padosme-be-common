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
	workers       []Worker
	logger        *zap.Logger
	wg            sync.WaitGroup
	stopCh        chan struct{}
	restartCh     chan restartRequest
	maxRestarts   int
	restartWait   time.Duration
	restartWindow time.Duration
	startOnce     sync.Once
	stopOnce      sync.Once
	panicsTotal   metric.Int64Counter
	restartsTotal metric.Int64Counter

	// nowFunc is used for testing to control time. Defaults to time.Now.
	nowFunc func() time.Time
}

type restartRequest struct {
	worker Worker
	err    error
}

// SupervisorConfig holds supervisor configuration
type SupervisorConfig struct {
	MaxRestarts   int           // Maximum restarts within the window (default: 5)
	RestartWait   time.Duration // Wait time before restart (default: 5s)
	RestartWindow time.Duration // Sliding window for counting restarts (default: 5min).
	// Restarts older than this are forgotten.
}

// NewSupervisor creates a new Supervisor with the given configuration
func NewSupervisor(logger *zap.Logger, config SupervisorConfig) *Supervisor {
	if config.MaxRestarts == 0 {
		config.MaxRestarts = 5
	}
	if config.RestartWait == 0 {
		config.RestartWait = 5 * time.Second
	}
	if config.RestartWindow == 0 {
		config.RestartWindow = 5 * time.Minute
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
		restartWindow: config.RestartWindow,
		panicsTotal:   panicsTotal,
		restartsTotal: restartsTotal,
		nowFunc:       time.Now,
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

// handleRestarts handles worker restart requests using a sliding window algorithm.
// Restarts older than RestartWindow are pruned before counting, so a worker that
// recovers and runs stably for longer than the window gets a fresh set of restart
// attempts.
func (s *Supervisor) handleRestarts(ctx context.Context) {
	defer s.wg.Done()

	restartTimestamps := make(map[string][]time.Time)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case req := <-s.restartCh:
			worker := req.worker
			now := s.nowFunc()

			// Append the current restart timestamp.
			restartTimestamps[worker.Name] = append(restartTimestamps[worker.Name], now)

			// Prune timestamps older than the restart window.
			cutoff := now.Add(-s.restartWindow)
			timestamps := restartTimestamps[worker.Name]
			pruned := timestamps[:0]
			for _, ts := range timestamps {
				if !ts.Before(cutoff) {
					pruned = append(pruned, ts)
				}
			}
			restartTimestamps[worker.Name] = pruned
			count := len(pruned)

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
