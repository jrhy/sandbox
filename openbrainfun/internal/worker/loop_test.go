package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunLoopRunsImmediatelyAndOnEachTick(t *testing.T) {
	ticks := make(chan time.Time, 1)
	runner := &fakeLoopRunner{calls: make(chan int, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runLoop(ctx, runner, ticks, nil)
		close(done)
	}()

	waitForLoopCall(t, runner.calls, 1)
	ticks <- time.Now()
	waitForLoopCall(t, runner.calls, 2)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runLoop did not stop after context cancellation")
	}
}

func TestRunLoopReportsErrorsAndContinues(t *testing.T) {
	ticks := make(chan time.Time, 1)
	runner := &fakeLoopRunner{
		calls: make(chan int, 2),
		errByCall: map[int]error{
			1: errors.New("first failure"),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu     sync.Mutex
		errors []string
	)
	done := make(chan struct{})
	go func() {
		runLoop(ctx, runner, ticks, func(err error) {
			mu.Lock()
			defer mu.Unlock()
			errors = append(errors, err.Error())
		})
		close(done)
	}()

	waitForLoopCall(t, runner.calls, 1)
	ticks <- time.Now()
	waitForLoopCall(t, runner.calls, 2)

	mu.Lock()
	defer mu.Unlock()
	if len(errors) != 1 || errors[0] != "first failure" {
		t.Fatalf("errors = %#v, want one reported error", errors)
	}

	cancel()
	<-done
}

type fakeLoopRunner struct {
	calls     chan int
	errByCall map[int]error
	count     int
}

func (f *fakeLoopRunner) RunOnce(ctx context.Context) error {
	f.count++
	f.calls <- f.count
	if err, ok := f.errByCall[f.count]; ok {
		return err
	}
	return nil
}

func waitForLoopCall(t *testing.T, calls <-chan int, want int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("call = %d, want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for call %d", want)
	}
}
