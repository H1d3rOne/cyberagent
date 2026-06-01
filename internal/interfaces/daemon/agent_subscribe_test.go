package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerAttachAndSubscribe(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(runtime.NewEngine(runtime.NewMemStore()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := mgr.Subscribe(ctx, "session-fake")
	require.NotNil(t, events)

	var wg sync.WaitGroup
	wg.Add(1)
	got := make([]string, 0, 4)
	go func() {
		defer wg.Done()
		for ev := range events {
			got = append(got, ev.Type)
		}
	}()

	// Drive a small deterministic turn through the engine.
	eng := mgr.engine
	_, _ = eng.Submit(ctx, runtime.Task{ID: "tx", Origin: "user", Goal: "hi"})
	time.Sleep(40 * time.Millisecond)

	cancel()
	wg.Wait()

	require.Contains(t, got, "task_created", "expected task_created event, got %v", got)
}

func TestAgentManagerSubscribeNilEngine(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	// No AttachEngine — should return a closed channel immediately.
	ch := mgr.Subscribe(context.Background(), "s")
	require.NotNil(t, ch)
	_, ok := <-ch
	require.False(t, ok, "expected closed channel when engine is nil")
}
