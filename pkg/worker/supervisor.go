package worker

import (
	"context"
	"sync"
	"time"

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
	workers     []Worker
	logger      *zap.Logger
	wg          sync.WaitGroup
	stopCh      chan struct{}
	restartCh   chan string
	maxRestarts int
	restartWait time.Duration
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

	return &Supervisor{
		workers:     make([]Worker, 0),
		logger:      logger,
		stopCh:      make(chan struct{}),
		restartCh:   make(chan string, 10),
		maxRestarts: config.MaxRestarts,
		restartWait: config.RestartWait,
	}
}

// AddWorker adds a worker to the supervisor
// Workers should be added before calling Start()
func (s *Supervisor) AddWorker(worker Worker) {
	s.workers = append(s.workers, worker)
}

// Start starts all workers with panic recovery and restart logic
func (s *Supervisor) Start(ctx context.Context) {
	s.logger.Info("starting supervisor", zap.Int("worker_count", len(s.workers)))

	// Start restart handler
	go s.handleRestarts(ctx)

	// Start all workers
	for _, worker := range s.workers {
		s.wg.Add(1)
		go s.runWorker(ctx, worker, 0)
	}
}

// Stop stops all workers gracefully
func (s *Supervisor) Stop() {
	s.logger.Info("stopping supervisor")
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("supervisor stopped")
}

// runWorker runs a worker with panic recovery and interval-based execution
func (s *Supervisor) runWorker(ctx context.Context, worker Worker, restartCount int) {
	defer s.wg.Done()

	s.logger.Info("starting worker",
		zap.String("worker", worker.Name),
		zap.Int("restart_count", restartCount),
	)

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("worker panic recovered",
				zap.String("worker", worker.Name),
				zap.Any("panic", r),
			)

			// Check if we should restart
			if restartCount < s.maxRestarts {
				select {
				case <-s.stopCh:
					return
				case s.restartCh <- worker.Name:
					// Request restart
				}
			} else {
				s.logger.Error("worker exceeded max restarts",
					zap.String("worker", worker.Name),
					zap.Int("max_restarts", s.maxRestarts),
				)
			}
		}
	}()

	// For workers with interval, run in a loop
	if worker.Interval > 0 {
		s.runIntervalWorker(ctx, worker)
	} else {
		// For one-time or continuous workers, just execute
		if err := s.safeExecute(ctx, worker); err != nil {
			s.logger.Error("worker execution error",
				zap.String("worker", worker.Name),
				zap.Error(err),
			)
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
			// Convert panic to error
			if e, ok := r.(error); ok {
				err = e
			}
		}
	}()

	return worker.Fn(ctx)
}

// handleRestarts handles worker restart requests
func (s *Supervisor) handleRestarts(ctx context.Context) {
	restartCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case workerName := <-s.restartCh:
			restartCounts[workerName]++
			count := restartCounts[workerName]

			if count > s.maxRestarts {
				s.logger.Error("worker permanently stopped",
					zap.String("worker", workerName),
					zap.Int("restart_count", count),
				)
				continue
			}

			// Wait before restart
			s.logger.Info("restarting worker",
				zap.String("worker", workerName),
				zap.Int("restart_count", count),
				zap.Duration("wait", s.restartWait),
			)

			time.Sleep(s.restartWait)

			// Find and restart the worker
			for _, worker := range s.workers {
				if worker.Name == workerName {
					s.wg.Add(1)
					go s.runWorker(ctx, worker, count)
					break
				}
			}
		}
	}
}
