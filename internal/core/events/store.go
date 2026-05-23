package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// Store persists append-only events as JSONL.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore creates an event store backed by path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the JSONL path backing this store.
func (s *Store) Path() string {
	return s.path
}

// Append writes one event as one JSON line.
func (s *Store) Append(event contracts.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create event store dir: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open event store: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}

// ReadAll reads all events from the store.
func (s *Store) ReadAll() ([]contracts.Event, error) {
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open event store: %w", err)
	}
	defer file.Close()

	var events []contracts.Event
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var event contracts.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode event line %d: %w", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan event store: %w", err)
	}
	return events, nil
}

// ReadByJob reads events for one job ID.
func (s *Store) ReadByJob(jobID string) ([]contracts.Event, error) {
	all, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	filtered := make([]contracts.Event, 0, len(all))
	for _, event := range all {
		if event.JobID == jobID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}
