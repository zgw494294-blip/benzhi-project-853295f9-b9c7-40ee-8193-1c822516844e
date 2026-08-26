package shallowbatchclone_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/policy"
	"collection-acclimatization-pass/internal/store"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestReturnedBatchCannotMutateStoredNestedState(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("打开仓储: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	next := 0
	newID := func(prefix string) string {
		next++
		return fmt.Sprintf("%s-%d", prefix, next)
	}
	service := application.NewService(
		repository,
		policy.Planner{NewID: newID},
		policy.Evaluator{MaxGap: 15 * time.Minute},
		fixedClock{now: now},
		newID,
	)

	created, err := service.CreateBatch(ctx, application.CreateBatchInput{
		VenueID:               "gallery-a",
		PlannedStartAt:        now.Add(24 * time.Hour),
		OwnerName:             "林文",
		VenueTemperatureRange: domain.Range{Min: 20, Max: 22},
		VenueHumidityRange:    domain.Range{Min: 45, Max: 50},
	}, "create-clone-test")
	if err != nil {
		t.Fatalf("创建批次: %v", err)
	}
	batchID := created.Batch.ID

	profiled, err := service.AddProfile(ctx, batchID, application.AddProfileInput{
		CollectionCode:         "OBJ-001",
		MaterialClasses:        []string{"纸", "木"},
		SensitivityLevel:       domain.SensitivityMedium,
		TargetTemperatureRange: domain.Range{Min: 19, Max: 23},
		TargetHumidityRange:    domain.Range{Min: 40, Max: 55},
		MaxTemperatureRate:     3,
		MaxHumidityRate:        12,
	}, application.Mutation{ExpectedVersion: created.Batch.Version, IdempotencyKey: "profile-clone-test"})
	if err != nil {
		t.Fatalf("登记材料档案: %v", err)
	}
	planned, err := service.GeneratePlan(ctx, batchID, application.Mutation{ExpectedVersion: profiled.Batch.Version, IdempotencyKey: "plan-clone-test"})
	if err != nil {
		t.Fatalf("生成方案: %v", err)
	}
	frozen, err := service.FreezePlan(ctx, batchID, planned.Batch.PlanDigest, application.Mutation{ExpectedVersion: planned.Batch.Version, IdempotencyKey: "freeze-clone-test"})
	if err != nil {
		t.Fatalf("冻结方案: %v", err)
	}
	_, err = service.StartBatch(ctx, batchID, application.Mutation{ExpectedVersion: frozen.Batch.Version, IdempotencyKey: "start-clone-test"})
	if err != nil {
		t.Fatalf("启动批次: %v", err)
	}

	returned, err := service.GetBatch(ctx, batchID)
	if err != nil {
		t.Fatalf("首次读取批次: %v", err)
	}
	if len(returned.Profiles) != 1 || len(returned.Profiles[0].MaterialClasses) == 0 {
		t.Fatalf("材料档案不完整: %+v", returned.Profiles)
	}
	if len(returned.Stages) == 0 || returned.Stages[0].StartedAt == nil {
		t.Fatalf("阶段启动证据不完整: %+v", returned.Stages)
	}
	originalMaterial := returned.Profiles[0].MaterialClasses[0]
	originalStartedAt := *returned.Stages[0].StartedAt
	returned.Profiles[0].MaterialClasses[0] = "调用方污染值"
	*returned.Stages[0].StartedAt = originalStartedAt.Add(2 * time.Hour)

	again, err := service.GetBatch(ctx, batchID)
	if err != nil {
		t.Fatalf("再次读取批次: %v", err)
	}
	gotMaterial := again.Profiles[0].MaterialClasses[0]
	gotStartedAt := *again.Stages[0].StartedAt
	if gotMaterial != originalMaterial || !gotStartedAt.Equal(originalStartedAt) {
		t.Fatalf("修改返回快照污染了仓储状态: material=%q startedAt=%s", gotMaterial, gotStartedAt.Format(time.RFC3339))
	}
}
