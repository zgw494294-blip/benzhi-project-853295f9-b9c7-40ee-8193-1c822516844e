package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"collection-acclimatization-pass/internal/domain"
)

type persisted struct {
	Batches     map[string]*batchRecord      `json:"batches"`
	Credentials map[string]*domainCredential `json:"credentials"`
	ByBatch     map[string]string            `json:"byBatch"`
	Idempotency map[string]idempotencyRecord `json:"idempotency"`
}
type batchRecord struct {
	Batch *domain.AcclimatizationBatch `json:"batch"`
}
type domainCredential struct {
	Credential *domain.AdmissionCredential `json:"credential"`
}
type idempotencyRecord struct {
	PayloadHash string  `json:"payloadHash"`
	Response    Receipt `json:"response"`
}
type SQLiteStore struct {
	mu    sync.Mutex
	path  string
	state persisted
}

func Open(_ context.Context, path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("数据库路径不能为空")
	}
	s := &SQLiteStore{path: path, state: persisted{Batches: map[string]*batchRecord{}, Credentials: map[string]*domainCredential{}, ByBatch: map[string]string{}, Idempotency: map[string]idempotencyRecord{}}}
	if path == ":memory:" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &s.state); err != nil {
			return nil, fmt.Errorf("恢复持久化数据: %w", err)
		}
	}
	if s.state.Batches == nil {
		s.state.Batches = map[string]*batchRecord{}
	}
	if s.state.Credentials == nil {
		s.state.Credentials = map[string]*domainCredential{}
	}
	if s.state.ByBatch == nil {
		s.state.ByBatch = map[string]string{}
	}
	if s.state.Idempotency == nil {
		s.state.Idempotency = map[string]idempotencyRecord{}
	}
	return s, nil
}
func (s *SQLiteStore) Close() error { return nil }

// readMergedLocked combines the current in-memory state with the latest contents
// persisted on disk. Records already present in memory (this handle's pending or
// just-applied writes) take precedence; records present only on disk (written by
// other handles opened against the same path) are preserved. This ensures that any
// commit order across multiple open handles keeps every successfully committed
// batch, credential and idempotency receipt.
func (s *SQLiteStore) readMergedLocked() (persisted, error) {
	merged := persisted{Batches: map[string]*batchRecord{}, Credentials: map[string]*domainCredential{}, ByBatch: map[string]string{}, Idempotency: map[string]idempotencyRecord{}}
	for k, v := range s.state.Batches {
		merged.Batches[k] = v
	}
	for k, v := range s.state.Credentials {
		merged.Credentials[k] = v
	}
	for k, v := range s.state.ByBatch {
		merged.ByBatch[k] = v
	}
	for k, v := range s.state.Idempotency {
		merged.Idempotency[k] = v
	}
	if s.path == ":memory:" {
		return merged, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return merged, nil
		}
		return persisted{}, err
	}
	if len(data) == 0 {
		return merged, nil
	}
	var disk persisted
	if err = json.Unmarshal(data, &disk); err != nil {
		return persisted{}, fmt.Errorf("合并持久化数据: %w", err)
	}
	if disk.Batches == nil {
		disk.Batches = map[string]*batchRecord{}
	}
	if disk.Credentials == nil {
		disk.Credentials = map[string]*domainCredential{}
	}
	if disk.ByBatch == nil {
		disk.ByBatch = map[string]string{}
	}
	if disk.Idempotency == nil {
		disk.Idempotency = map[string]idempotencyRecord{}
	}
	for k, v := range disk.Batches {
		if _, ok := merged.Batches[k]; !ok {
			merged.Batches[k] = v
		}
	}
	for k, v := range disk.Credentials {
		if _, ok := merged.Credentials[k]; !ok {
			merged.Credentials[k] = v
		}
	}
	for k, v := range disk.ByBatch {
		if _, ok := merged.ByBatch[k]; !ok {
			merged.ByBatch[k] = v
		}
	}
	for k, v := range disk.Idempotency {
		if _, ok := merged.Idempotency[k]; !ok {
			merged.Idempotency[k] = v
		}
	}
	return merged, nil
}
func (s *SQLiteStore) commitLocked() error {
	if s.path == ":memory:" {
		return nil
	}
	merged, err := s.readMergedLocked()
	if err != nil {
		return err
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	tmp := s.path + ".wal.tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Refresh the in-memory state with the merged result so subsequent reads and
	// version checks through this handle observe records persisted by other handles.
	s.state = merged
	return nil
}
