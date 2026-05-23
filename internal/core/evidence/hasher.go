package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type HashRecord struct {
	EvidenceID string `json:"evidence_id"`
	Hash       string `json:"hash"`
	Duplicate  bool   `json:"duplicate"`
}

type Hasher struct {
	seen map[string]string
}

func NewHasher() *Hasher {
	return &Hasher{seen: make(map[string]string)}
}

func (h *Hasher) Hash(e contracts.Evidence) HashRecord {
	hash := computeEvidenceHash(e)
	record := HashRecord{EvidenceID: e.ID, Hash: hash}
	if existingID, ok := h.seen[hash]; ok && existingID != e.ID {
		record.Duplicate = true
	} else {
		h.seen[hash] = e.ID
	}
	return record
}

func (h *Hasher) Dedup(evidence []contracts.Evidence) (unique []contracts.Evidence, duplicates []HashRecord) {
	for _, e := range evidence {
		record := h.Hash(e)
		if record.Duplicate {
			duplicates = append(duplicates, record)
		} else {
			unique = append(unique, e)
		}
	}
	return unique, duplicates
}

func (h *Hasher) IsDuplicate(e contracts.Evidence) bool {
	hash := computeEvidenceHash(e)
	existingID, ok := h.seen[hash]
	return ok && existingID != e.ID
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func computeEvidenceHash(e contracts.Evidence) string {
	hasher := sha256.New()
	hasher.Write([]byte(e.Type))
	hasher.Write([]byte(e.TargetRef))
	hasher.Write([]byte(e.Title))
	if e.Data != nil {
		keys := sortedKeys(e.Data)
		for _, k := range keys {
			hasher.Write([]byte(k))
			v, _ := json.Marshal(e.Data[k])
			hasher.Write(v)
		}
	}
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(hasher.Sum(nil)))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
