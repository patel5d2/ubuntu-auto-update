package events

import (
	"sync"
	"testing"
	"time"
)

func TestBroker_FanoutDelivery(t *testing.T) {
	b := NewBroker()
	a, releaseA := b.Subscribe()
	c, releaseC := b.Subscribe()
	t.Cleanup(releaseA)
	t.Cleanup(releaseC)

	want := Event{Table: "hosts", Op: "UPDATE", ID: 42}
	b.Publish(want)

	for i, ch := range []<-chan Event{a, c} {
		select {
		case got := <-ch:
			if got != want {
				t.Errorf("subscriber %d: got %+v, want %+v", i, got, want)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestBroker_ReleaseStopsDelivery(t *testing.T) {
	b := NewBroker()
	ch, release := b.Subscribe()
	if got, want := b.SubscriberCount(), 1; got != want {
		t.Fatalf("count after subscribe: got %d, want %d", got, want)
	}
	release()
	if got, want := b.SubscriberCount(), 0; got != want {
		t.Fatalf("count after release: got %d, want %d", got, want)
	}

	// Publishing after release must not panic and the channel must not
	// receive anything new.
	b.Publish(Event{Table: "hosts", Op: "UPDATE", ID: 1})
	select {
	case ev, ok := <-ch:
		if ok {
			t.Errorf("got event %+v after release; want no delivery", ev)
		}
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery.
	}
}

func TestBroker_SlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBroker()
	_, release := b.Subscribe() // never read from this one
	t.Cleanup(release)

	// Fill past the buffer; if Publish blocked we'd deadlock.
	done := make(chan struct{})
	go func() {
		for i := 0; i < SubscriberBufferSize+10; i++ {
			b.Publish(Event{Table: "hosts", Op: "UPDATE", ID: int64(i)})
		}
		close(done)
	}()

	select {
	case <-done:
		// pass
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked on slow subscriber")
	}

	if got := b.DroppedCount(); got == 0 {
		t.Errorf("expected at least one dropped event, got %d", got)
	}
}

func TestBroker_ConcurrentPublishSubscribe(t *testing.T) {
	// Smoke test: multiple goroutines publishing while subscribers come and
	// go must not race. Run with `go test -race` to actually catch issues.
	b := NewBroker()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.Publish(Event{Table: "hosts", Op: "UPDATE", ID: 1})
				}
			}
		}()
	}

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ch, release := b.Subscribe()
				select {
				case <-ch:
				case <-time.After(10 * time.Millisecond):
				}
				release()
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
