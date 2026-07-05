package ssh

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitWithAbort_CompletesBeforeTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	want := errors.New("exit status 1")
	err, timedOut := WaitWithAbort(ctx, func() error { return want }, func() {
		t.Fatal("abort must not be called when wait finishes in time")
	})
	if timedOut {
		t.Fatal("timedOut = true, want false")
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestWaitWithAbort_TimesOutAndAborts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// Simulate a hung remote command: wait blocks until abort unblocks it,
	// exactly like a session read that only returns once the session closes.
	unblock := make(chan struct{})
	aborted := false
	err, timedOut := WaitWithAbort(ctx,
		func() error { <-unblock; return errors.New("session closed") },
		func() { aborted = true; close(unblock) },
	)
	if !timedOut {
		t.Fatal("timedOut = false, want true")
	}
	if !aborted {
		t.Fatal("abort was not called")
	}
	if err == nil {
		t.Fatal("expected the unblocked wait error to be returned")
	}
}
