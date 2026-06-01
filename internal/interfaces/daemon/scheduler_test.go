package daemon

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestScheduleManagerPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newScheduleManager(dir)
	spec, err := m.Create(scheduleSpec{
		Name:    "round-trip",
		Cron:    "*/5 * * * *",
		Action:  scheduleActionScanWeb,
		Target:  "https://example.com",
		Scope:   "subdomain",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if spec.ID == "" {
		t.Fatal("expected ID assigned")
	}

	reloaded := newScheduleManager(dir)
	got, err := reloaded.Get(spec.ID)
	if err != nil {
		t.Fatalf("reload Get error: %v", err)
	}
	if got.Cron != spec.Cron || got.Target != spec.Target || got.Action != spec.Action {
		t.Fatalf("reloaded spec mismatch: got=%+v want=%+v", got, spec)
	}
	if got.InFlight {
		t.Fatal("InFlight should reset to false after reload")
	}
}

func TestScheduleManagerSkipIfRunning(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	var (
		started = make(chan struct{})
		release = make(chan struct{})
		mu      sync.Mutex
		fires   int
	)
	m.BindRunner(scheduleRunner{
		StartScanWeb: func(ctx context.Context, s scheduleSpec) (string, error) {
			mu.Lock()
			fires++
			mu.Unlock()
			close(started)
			<-release
			return "job_xxx", nil
		},
	})
	spec, err := m.Create(scheduleSpec{
		Name:   "slow",
		Cron:   "*/5 * * * *",
		Action: scheduleActionScanWeb,
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	go m.fire(context.Background(), spec.ID)
	<-started

	m.fire(context.Background(), spec.ID)

	got, err := m.Get(spec.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.LastStatus != "skipped" {
		t.Fatalf("LastStatus = %q, want skipped", got.LastStatus)
	}

	close(release)
	deadline := time.After(2 * time.Second)
	for {
		got, err = m.Get(spec.ID)
		if err != nil {
			t.Fatalf("Get error: %v", err)
		}
		if got.LastStatus == "success" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("first run did not finish: %+v", got)
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if fires != 1 {
		t.Fatalf("runner called %d times, want 1 (skip should not invoke runner)", fires)
	}
}

func TestScheduleManagerInvalidCronRejected(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	if _, err := m.Create(scheduleSpec{
		Name:    "bad",
		Cron:    "this is not cron",
		Action:  scheduleActionScanWeb,
		Target:  "https://example.com",
	}); err == nil {
		t.Fatal("expected error for invalid cron")
	}
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expected no schedules persisted, got %d", len(list))
	}
}

func TestScheduleManagerEnableDisableTogglesEntry(t *testing.T) {
	m := newScheduleManager(t.TempDir())
	m.Start()
	defer m.Stop(nil)

	spec, err := m.Create(scheduleSpec{
		Name:    "toggle",
		Cron:    "*/5 * * * *",
		Action:  scheduleActionScanWeb,
		Target:  "https://example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(m.cron.Entries()) != 1 {
		t.Fatalf("expected 1 cron entry after enabled create, got %d", len(m.cron.Entries()))
	}

	if _, err := m.SetEnabled(spec.ID, false); err != nil {
		t.Fatalf("disable error: %v", err)
	}
	if len(m.cron.Entries()) != 0 {
		t.Fatalf("expected 0 cron entries after disable, got %d", len(m.cron.Entries()))
	}

	if _, err := m.SetEnabled(spec.ID, true); err != nil {
		t.Fatalf("re-enable error: %v", err)
	}
	if len(m.cron.Entries()) != 1 {
		t.Fatalf("expected 1 cron entry after re-enable, got %d", len(m.cron.Entries()))
	}
}
