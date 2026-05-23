package evidence

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestHasherDedup(t *testing.T) {
	h := NewHasher()

	e1 := contracts.Evidence{
		ID:        "evidence_001",
		Type:      contracts.EvidenceHTTPResponse,
		Title:     "HTTP 200",
		TargetRef: "http://localhost:8080/",
		Data:      map[string]any{"status_code": 200},
	}
	e2 := contracts.Evidence{
		ID:        "evidence_002",
		Type:      contracts.EvidenceHTTPResponse,
		Title:     "HTTP 200",
		TargetRef: "http://localhost:8080/",
		Data:      map[string]any{"status_code": 200},
	}
	e3 := contracts.Evidence{
		ID:        "evidence_003",
		Type:      contracts.EvidenceHTTPHeader,
		Title:     "Missing HSTS",
		TargetRef: "http://localhost:8080/",
		Data:      map[string]any{"header": "Strict-Transport-Security"},
	}

	unique, duplicates := h.Dedup([]contracts.Evidence{e1, e2, e3})
	if len(unique) != 2 {
		t.Errorf("expected 2 unique, got %d", len(unique))
	}
	if len(duplicates) != 1 {
		t.Errorf("expected 1 duplicate, got %d", len(duplicates))
	}
	if duplicates[0].EvidenceID != "evidence_002" {
		t.Errorf("expected duplicate to be evidence_002, got %s", duplicates[0].EvidenceID)
	}
}

func TestHasherIsDuplicate(t *testing.T) {
	h := NewHasher()

	e1 := contracts.Evidence{
		ID:        "evidence_001",
		Type:      contracts.EvidenceJSRoute,
		Title:     "API route",
		TargetRef: "http://localhost/api/users",
		Data:      map[string]any{"raw": "/api/users"},
	}

	record := h.Hash(e1)
	if record.Duplicate {
		t.Error("first evidence should not be duplicate")
	}

	if h.IsDuplicate(e1) {
		t.Error("same evidence should not be duplicate of itself")
	}

	e2 := contracts.Evidence{
		ID:        "evidence_002",
		Type:      contracts.EvidenceJSRoute,
		Title:     "API route",
		TargetRef: "http://localhost/api/users",
		Data:      map[string]any{"raw": "/api/users"},
	}
	if !h.IsDuplicate(e2) {
		t.Error("identical evidence with different ID should be duplicate")
	}
}

func TestHashBytes(t *testing.T) {
	hash1 := HashBytes([]byte("hello"))
	hash2 := HashBytes([]byte("hello"))
	hash3 := HashBytes([]byte("world"))

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different input should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64 char hex hash, got %d chars", len(hash1))
	}
}
