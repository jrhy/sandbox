package worker

import (
	"context"
	"time"
)

type loopRunner interface {
	RunOnce(ctx context.Context) error
}

func RunLoop(ctx context.Context, runner loopRunner, pollInterval time.Duration, reportError func(error)) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	runLoop(ctx, runner, ticker.C, reportError)
}

func runLoop(ctx context.Context, runner loopRunner, ticks <-chan time.Time, reportError func(error)) {
	if runner == nil {
		return
	}

	runOnce := func() {
		if err := runner.RunOnce(ctx); err != nil && reportError != nil {
			reportError(err)
		}
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			runOnce()
		}
	}
}
