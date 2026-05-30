package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

func TestProviderStoreLoadMissing(t *testing.T) {
	store := newProviderStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected empty providers, got %d", len(cfg.Providers))
	}
}

func TestProviderStoreSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	store := newProviderStore(path)
	want := web.ProviderConfig{
		Providers: []web.ProviderProfile{{ID: "p1", Name: "OpenAI", Type: "openai", Model: "gpt-4o", APIKey: "sk-x"}},
		ActiveID:  "p1",
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %v", info.Mode().Perm())
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveID != "p1" || len(got.Providers) != 1 || got.Providers[0].APIKey != "sk-x" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
