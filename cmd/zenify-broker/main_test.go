package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// The spec allows each login at most 2 concurrent /query children.
func TestMaxConcurrentPerLoginIsTwo(t *testing.T) {
	if maxConcurrentPerLogin != 2 {
		t.Fatalf("maxConcurrentPerLogin = %d, want 2", maxConcurrentPerLogin)
	}
}

func TestWatchSignals_SecondSignalExits(t *testing.T) {
	sigs := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	exited := make(chan int, 1)
	go watchSignals(sigs, cancel, func(code int) { exited <- code })

	sigs <- syscall.SIGTERM
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first signal did not start the shutdown")
	}
	select {
	case <-exited:
		t.Fatal("first signal must not exit")
	case <-time.After(50 * time.Millisecond):
	}
	sigs <- syscall.SIGINT
	select {
	case code := <-exited:
		if code == 0 {
			t.Fatal("exit code 0, want non-zero")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second signal did not exit")
	}
}
