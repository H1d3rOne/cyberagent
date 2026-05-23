package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketJobs  = []byte("jobs")
	bucketPlans = []byte("plans")
	bucketSteps = []byte("steps")
)

// ErrNotFound is returned when a state object does not exist.
var ErrNotFound = errors.New("state object not found")

// Store persists current state snapshots in a bbolt database.
type Store struct {
	db *bolt.DB
}

// Open opens or creates a bbolt-backed state store.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bbolt state store: %w", err)
	}
	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) init() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{bucketJobs, bucketPlans, bucketSteps} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveJob saves a job snapshot.
func (s *Store) SaveJob(job contracts.Job) error {
	return putJSON(s.db, bucketJobs, job.ID, job)
}

// LoadJob loads a job snapshot.
func (s *Store) LoadJob(id string) (contracts.Job, error) {
	var job contracts.Job
	err := getJSON(s.db, bucketJobs, id, &job)
	return job, err
}

// SavePlan saves a plan snapshot.
func (s *Store) SavePlan(plan contracts.Plan) error {
	return putJSON(s.db, bucketPlans, plan.ID, plan)
}

// LoadPlan loads a plan snapshot.
func (s *Store) LoadPlan(id string) (contracts.Plan, error) {
	var plan contracts.Plan
	err := getJSON(s.db, bucketPlans, id, &plan)
	return plan, err
}

// SaveStep saves a step snapshot.
func (s *Store) SaveStep(step contracts.Step) error {
	return putJSON(s.db, bucketSteps, step.ID, step)
}

// LoadStep loads a step snapshot.
func (s *Store) LoadStep(id string) (contracts.Step, error) {
	var step contracts.Step
	err := getJSON(s.db, bucketSteps, id, &step)
	return step, err
}

// ListSteps returns all steps for a job ID.
func (s *Store) ListSteps(jobID string) ([]contracts.Step, error) {
	steps := []contracts.Step{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSteps)
		if bucket == nil {
			return fmt.Errorf("bucket %s missing", bucketSteps)
		}
		return bucket.ForEach(func(_, value []byte) error {
			var step contracts.Step
			if err := json.Unmarshal(value, &step); err != nil {
				return fmt.Errorf("decode step: %w", err)
			}
			if step.JobID == jobID {
				steps = append(steps, step)
			}
			return nil
		})
	})
	return steps, err
}

func putJSON(db *bolt.DB, bucketName []byte, key string, value any) error {
	if key == "" {
		return fmt.Errorf("empty key for bucket %s", bucketName)
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state object: %w", err)
	}
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("bucket %s missing", bucketName)
		}
		return bucket.Put([]byte(key), encoded)
	})
}

func getJSON(db *bolt.DB, bucketName []byte, key string, out any) error {
	if key == "" {
		return fmt.Errorf("empty key for bucket %s", bucketName)
	}
	return db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("bucket %s missing", bucketName)
		}
		value := bucket.Get([]byte(key))
		if value == nil {
			return fmt.Errorf("%w: %s/%s", ErrNotFound, bucketName, key)
		}
		if err := json.Unmarshal(value, out); err != nil {
			return fmt.Errorf("decode state object: %w", err)
		}
		return nil
	})
}
