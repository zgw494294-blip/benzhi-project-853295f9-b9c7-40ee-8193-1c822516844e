package commitfailurerollback_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

func TestFailedCommitDoesNotLeakIntoMemory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.json")
	repository, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dbPath, 0o700); err != nil {
		t.Fatal(err)
	}
	batch, err := domain.NewBatch(
		"batch-rollback", "venue-a", "保管员",
		time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		domain.Range{Min: 20, Max: 22}, domain.Range{Min: 45, Max: 50},
		time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, commitErr := repository.CreateBatch(context.Background(), "create-rollback", "payload", batch)
	_, getErr := repository.GetBatch(context.Background(), batch.ID)
	if commitErr == nil || getErr == nil {
		t.Fatalf("提交失败后的内存状态未回滚: commitErr=%v getErr=%v", commitErr, getErr)
	}
}
