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
func (s *SQLiteStore) commitLocked() error {
	if s.path == ":memory:" {
		return nil
	}
	data, err := json.Marshal(s.state)
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
	return nil
}
