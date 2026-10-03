package extraction

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
)

type blockingTextEncoder struct {
	started            chan struct{}
	release            chan struct{}
	encodeReturned     chan struct{}
	closeCalls         atomic.Int32
	closedBeforeEncode atomic.Bool
}

type contextAwareTextEncoder struct {
	started        chan struct{}
	encodeReturned chan struct{}
	cancelObserved chan struct{}
	closeCalls     atomic.Int32
}

func (m *contextAwareTextEncoder) Encode(ctx context.Context, _ string, _ int) (textencoding.Response, error) {
	close(m.started)
	<-ctx.Done()
	close(m.cancelObserved)
	close(m.encodeReturned)
	return textencoding.Response{}, ctx.Err()
}

func (m *contextAwareTextEncoder) Close() error {
	select {
	case <-m.encodeReturned:
	default:
		return errors.New("finalized before Encode returned")
	}
	m.closeCalls.Add(1)
	return nil
}

func (m *blockingTextEncoder) Encode(context.Context, string, int) (textencoding.Response, error) {
	close(m.started)
	<-m.release
	close(m.encodeReturned)
	return textencoding.Response{}, errors.New("fake encode complete")
}

func (m *blockingTextEncoder) Close() error {
	if m.encodeReturned == nil {
		m.closedBeforeEncode.Store(true)
	} else {
		select {
		case <-m.encodeReturned:
		default:
			m.closedBeforeEncode.Store(true)
		}
	}
	m.closeCalls.Add(1)
	return nil
}

func TestCybertronEmbedderCloseWaitsForActiveEncodeAndRejectsNew(t *testing.T) {
	model := &blockingTextEncoder{
		started:        make(chan struct{}),
		release:        make(chan struct{}),
		encodeReturned: make(chan struct{}),
	}
	embedder := &CybertronEmbedder{
		modelPool: make(chan textencoding.Interface, 1),
		allModels: []textencoding.Interface{model},
		dimension: 2,
	}
	embedder.modelPool <- model

	type encodeOutcome struct {
		err   error
		panic any
	}
	encodeDone := make(chan encodeOutcome, 1)
	go func() {
		outcome := encodeOutcome{}
		defer func() {
			outcome.panic = recover()
			encodeDone <- outcome
		}()
		_, outcome.err = embedder.Encode(context.Background(), "in flight")
	}()

	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("Encode did not acquire and call the fake model")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- embedder.Close() }()
	closedDeadline := time.Now().Add(time.Second)
	for {
		embedder.mu.Lock()
		closed := embedder.closed
		embedder.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(closedDeadline) {
			t.Fatal("Close did not mark the embedder closed")
		}
		time.Sleep(time.Millisecond)
	}

	if _, err := embedder.Encode(context.Background(), "after close"); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("Encode after Close error = %v, want closed error", err)
	}
	if _, err := embedder.Encode(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("empty Encode after Close error = %v, want closed error", err)
	}

	closeReturnedEarly := false
	select {
	case err := <-closeDone:
		closeReturnedEarly = true
		t.Errorf("Close returned before the active Encode completed: %v", err)
	case <-time.After(2500 * time.Millisecond):
	}

	close(model.release)
	select {
	case outcome := <-encodeDone:
		if outcome.panic != nil {
			t.Errorf("Encode panicked while returning its model: %v", outcome.panic)
		}
		if outcome.err == nil || !strings.Contains(outcome.err.Error(), "encoding failed") {
			t.Errorf("Encode error = %v, want wrapped fake model error", outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("active Encode did not finish after release")
	}

	if !closeReturnedEarly {
		select {
		case err := <-closeDone:
			if err != nil {
				t.Errorf("Close returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Close did not finish after active Encode completed")
		}
	}
	if model.closedBeforeEncode.Load() {
		t.Error("Close finalized the model before the active Encode returned")
	}
	if got := model.closeCalls.Load(); got != 1 {
		t.Errorf("model Close calls = %d, want 1", got)
	}
}

func TestCybertronEmbedderCloseContextReturnsOnDeadlineAndFinalizesAfterEncode(t *testing.T) {
	model := &blockingTextEncoder{
		started:        make(chan struct{}),
		release:        make(chan struct{}),
		encodeReturned: make(chan struct{}),
	}
	embedder := &CybertronEmbedder{
		modelPool: make(chan textencoding.Interface, 1),
		allModels: []textencoding.Interface{model},
		dimension: 2,
	}
	embedder.modelPool <- model

	encodeDone := make(chan struct{})
	go func() {
		defer close(encodeDone)
		_, _ = embedder.Encode(context.Background(), "in flight")
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("Encode did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := embedder.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseContext error = %v, want deadline exceeded", err)
	}
	if got := model.closeCalls.Load(); got != 0 {
		t.Fatalf("model finalized %d times before Encode returned", got)
	}

	close(model.release)
	select {
	case <-encodeDone:
	case <-time.After(time.Second):
		t.Fatal("Encode did not finish after release")
	}
	deadline := time.Now().Add(time.Second)
	for model.closeCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if model.closedBeforeEncode.Load() {
		t.Fatal("model finalized before Encode returned")
	}
	if got := model.closeCalls.Load(); got != 1 {
		t.Fatalf("model Close calls = %d, want 1", got)
	}
}

func TestCybertronEmbedderConcurrentCloseContextAndCloseShareCleanup(t *testing.T) {
	model := &blockingTextEncoder{
		started:        make(chan struct{}),
		release:        make(chan struct{}),
		encodeReturned: make(chan struct{}),
	}
	embedder := &CybertronEmbedder{
		modelPool: make(chan textencoding.Interface, 1),
		allModels: []textencoding.Interface{model},
		dimension: 2,
	}
	embedder.modelPool <- model

	go func() { _, _ = embedder.Encode(context.Background(), "in flight") }()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("Encode did not start")
	}

	closeReturned := make(chan error, 1)
	go func() { closeReturned <- embedder.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := embedder.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseContext error = %v, want deadline exceeded", err)
	}
	close(model.release)
	select {
	case err := <-closeReturned:
		if err != nil {
			t.Fatalf("Close error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after Encode returned")
	}
	if got := model.closeCalls.Load(); got != 1 {
		t.Fatalf("model Close calls = %d, want 1", got)
	}
}

func TestCybertronEmbedderCloseCancelsActiveEncodeBeforeFinalizing(t *testing.T) {
	model := &contextAwareTextEncoder{
		started:        make(chan struct{}),
		encodeReturned: make(chan struct{}),
		cancelObserved: make(chan struct{}),
	}
	embedder := &CybertronEmbedder{
		modelPool: make(chan textencoding.Interface, 1),
		allModels: []textencoding.Interface{model},
		dimension: 2,
	}
	embedder.modelPool <- model

	encodeDone := make(chan error, 1)
	go func() {
		_, err := embedder.Encode(context.Background(), "cancellable")
		encodeDone <- err
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("Encode did not start")
	}

	if err := embedder.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	select {
	case <-model.cancelObserved:
	default:
		t.Fatal("Close did not cancel the active Encode context")
	}
	if err := <-encodeDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Encode error = %v, want context canceled", err)
	}
	if got := model.closeCalls.Load(); got != 1 {
		t.Fatalf("model Close calls = %d, want 1", got)
	}
}
