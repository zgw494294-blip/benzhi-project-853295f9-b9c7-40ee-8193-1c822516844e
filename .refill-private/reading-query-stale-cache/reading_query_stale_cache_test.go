package reading_query_stale_cache_test

import (
	"context"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/policy"
	"collection-acclimatization-pass/internal/store"
)

func TestReadingQueryCacheTracksCommittedBatchVersion(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("打开内存仓储: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	base := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	batch := &domain.AcclimatizationBatch{
		ID:                    "batch-cache-version",
		VenueID:               "venue-a",
		OwnerName:             "保护员甲",
		PlannedStartAt:        base,
		VenueTemperatureRange: domain.Range{Min: 18, Max: 24},
		VenueHumidityRange:    domain.Range{Min: 40, Max: 60},
		Status:                domain.BatchRunning,
		CurrentStageID:        "stage-a",
		Version:               1,
		CreatedAt:             base,
		UpdatedAt:             base,
		Profiles: []domain.ObjectMaterialProfile{{
			ID: "object-a", BatchID: "batch-cache-version", CollectionCode: "COL-1",
			MaterialClasses: []string{"纸张"}, SensitivityLevel: domain.SensitivityHigh,
			TargetTemperatureRange: domain.Range{Min: 18, Max: 24}, TargetHumidityRange: domain.Range{Min: 40, Max: 60},
			MaxTemperatureRate: 2, MaxHumidityRate: 5,
		}},
		Stages: []domain.AcclimatizationStage{{
			ID: "stage-a", BatchID: "batch-cache-version", Sequence: 1,
			TargetTemperatureRange: domain.Range{Min: 18, Max: 24}, TargetHumidityRange: domain.Range{Min: 40, Max: 60},
			MinimumDuration: 30 * time.Minute, StabilityWindow: 10 * time.Minute,
			Status: domain.StageActive, Attempt: 1, StartedAt: timePointer(base),
		}},
		Readings: []domain.Reading{{
			ID: "reading-initial", BatchID: "batch-cache-version", StageID: "stage-a", Attempt: 1,
			ObservedAt: base, Temperature: 21, Humidity: 50, Verdict: "collecting",
		}},
	}
	if _, _, err = repository.CreateBatch(ctx, "create-cache-version", "payload", batch); err != nil {
		t.Fatalf("创建测试批次: %v", err)
	}

	service := application.NewService(repository, nil, policy.Evaluator{}, nil, func(string) string { return "reading-committed" })
	query := application.ReadingQuery{StageID: "stage-a", Attempt: 1}
	before, err := service.QueryReadings(ctx, batch.ID, query)
	if err != nil {
		t.Fatalf("首次查询读数: %v", err)
	}
	if before.Stats.SampleCount != 1 {
		t.Fatalf("测试前置条件错误: sampleCount=%d", before.Stats.SampleCount)
	}

	_, err = service.SubmitReading(ctx, batch.ID, application.ReadingInput{
		StageID: "stage-a", ObservedAt: base.Add(5 * time.Minute), Temperature: 21, Humidity: 50,
	}, application.Mutation{ExpectedVersion: 1, IdempotencyKey: "append-reading"})
	if err != nil {
		t.Fatalf("提交第二条读数: %v", err)
	}

	after, err := service.QueryReadings(ctx, batch.ID, query)
	if err != nil {
		t.Fatalf("写入后查询读数: %v", err)
	}
	if after.Stats.SampleCount != 2 || len(after.Items) != 2 {
		t.Fatalf("批次已提交新版本后仍返回旧缓存: sampleCount=%d items=%d", after.Stats.SampleCount, len(after.Items))
	}
}

func timePointer(value time.Time) *time.Time { return &value }
