package events

import (
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestStoreAppendReadAllAndReadByJob(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "events.jsonl"))
	e1 := contracts.NewEvent("event_001", "job_001", "step_001", contracts.EventStepStarted, contracts.EventLevelInfo, "started")
	e2 := contracts.NewEvent("event_002", "job_002", "step_002", contracts.EventStepStarted, contracts.EventLevelInfo, "started")
	if err := store.Append(e1); err != nil {
		t.Fatalf("Append(e1) error = %v", err)
	}
	if err := store.Append(e2); err != nil {
		t.Fatalf("Append(e2) error = %v", err)
	}

	all, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(ReadAll()) = %d, want 2", len(all))
	}

	jobEvents, err := store.ReadByJob("job_001")
	if err != nil {
		t.Fatalf("ReadByJob() error = %v", err)
	}
	if len(jobEvents) != 1 || jobEvents[0].ID != "event_001" {
		t.Fatalf("unexpected job events: %#v", jobEvents)
	}
}

func TestBusPublishBroadcastsAndStores(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "events.jsonl"))
	bus := NewBus(store)
	ch, cancel := bus.Subscribe(1)
	defer cancel()

	event := contracts.NewEvent("event_001", "job_001", "", contracts.EventJobCreated, contracts.EventLevelInfo, "created")
	if err := bus.Publish(event); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case got := <-ch:
		if got.ID != event.ID {
			t.Fatalf("subscriber got %#v, want %#v", got, event)
		}
	default:
		t.Fatalf("expected subscriber event")
	}

	all, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(all) != 1 || all[0].ID != event.ID {
		t.Fatalf("unexpected persisted events: %#v", all)
	}
}
