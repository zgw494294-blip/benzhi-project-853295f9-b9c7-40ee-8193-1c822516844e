package canceledcontextwrite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

func TestCanceledContextCannotCreateBatch(t *testing.T) {
	repository, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := domain.NewBatch(
		"batch-canceled", "venue-a", "保管员",
		time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		domain.Range{Min: 20, Max: 22}, domain.Range{Min: 45, Max: 50},
		time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, createErr := repository.CreateBatch(canceled, "create-canceled", "payload", batch)
	_, getErr := repository.GetBatch(context.Background(), batch.ID)
	if !errors.Is(createErr, context.Canceled) || getErr == nil {
		t.Fatalf("已取消的请求仍写入批次: createErr=%v getErr=%v", createErr, getErr)
	}
}
