package stalestoreoverwrite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

func TestStaleStoreHandleCannotOverwriteCommittedBatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.json")
	first, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	newBatch := func(id string) *domain.AcclimatizationBatch {
		batch, createErr := domain.NewBatch(id, "venue-a", "保管员", now.Add(time.Hour), domain.Range{Min: 20, Max: 22}, domain.Range{Min: 45, Max: 50}, now)
		if createErr != nil {
			t.Fatal(createErr)
		}
		return batch
	}
	if _, _, err := first.CreateBatch(context.Background(), "first", "first", newBatch("batch-first")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := second.CreateBatch(context.Background(), "second", "second", newBatch("batch-second")); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	batches, err := reopened.ListBatches(context.Background(), store.BatchFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("陈旧持久化快照覆盖了已提交批次: got=%d want=2", len(batches))
	}
}
