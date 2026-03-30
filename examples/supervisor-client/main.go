package main

import (
	"context"
	"errors"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kaaslabs/padosme-be-common/v4/pkg/worker"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	supervisor := worker.NewSupervisor(logger, worker.SupervisorConfig{
		MaxRestarts: 3,
		RestartWait: 2 * time.Second,
	})

	var streamAttempts atomic.Int32
	supervisor.AddWorker(worker.Worker{
		Name:     "order-stream-consumer",
		Interval: 0,
		Fn: func(ctx context.Context) error {
			attempt := streamAttempts.Add(1)
			logger.Info("stream worker started", zap.Int32("attempt", attempt))

			// Demonstrate restart behavior for long-running workers.
			if attempt <= 2 {
				return errors.New("simulated broker disconnect")
			}

			for {
				select {
				case <-ctx.Done():
					logger.Info("stream worker shutting down")
					return ctx.Err()
				case <-time.After(5 * time.Second):
					logger.Info("stream worker heartbeat")
				}
			}
		},
	})

	supervisor.AddWorker(worker.Worker{
		Name:     "session-cleanup",
		Interval: 30 * time.Second,
		Fn: func(ctx context.Context) error {
			logger.Info("running cleanup batch")
			return nil
		},
	})

	supervisor.Start(ctx)
	<-ctx.Done()
	supervisor.Stop()
}
