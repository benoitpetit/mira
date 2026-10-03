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
