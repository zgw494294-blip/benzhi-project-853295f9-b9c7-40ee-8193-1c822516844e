package plancacheidentity_test

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

func TestPlanCacheDoesNotReuseAggregateIdentity(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	sequence := 0
	newID := func(prefix string) string {
		sequence++
		return fmt.Sprintf("%s-%d", prefix, sequence)
	}
	clock := fixedClock{now: time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)}
	service := application.NewService(repository, &policy.Planner{NewID: newID}, policy.Evaluator{}, clock, newID)

	first := prepareBatch(t, ctx, service, "first")
	second := prepareBatch(t, ctx, service, "second")

	if first.Stages[0].ID == second.Stages[0].ID || second.Stages[0].BatchID != second.ID {
		t.Fatalf("TestPlanCacheDoesNotReuseAggregateIdentity: 第二批次复用了第一批次阶段身份：firstStage=%s secondStage=%s secondStageBatch=%s secondBatch=%s", first.Stages[0].ID, second.Stages[0].ID, second.Stages[0].BatchID, second.ID)
	}
}

func prepareBatch(t *testing.T, ctx context.Context, service *application.Service, key string) *domain.AcclimatizationBatch {
	t.Helper()
	created, err := service.CreateBatch(ctx, application.CreateBatchInput{
		VenueID:               "gallery-cache",
		PlannedStartAt:        time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		OwnerName:             "保护员",
		VenueTemperatureRange: domain.Range{Min: 20, Max: 22},
		VenueHumidityRange:    domain.Range{Min: 45, Max: 50},
	}, "create-"+key)
	if err != nil {
		t.Fatal(err)
	}
	profiled, err := service.AddProfile(ctx, created.Batch.ID, application.AddProfileInput{
		CollectionCode:         "OBJECT-" + key,
		MaterialClasses:        []string{"纸"},
		SensitivityLevel:       domain.SensitivityMedium,
		TargetTemperatureRange: domain.Range{Min: 20, Max: 22},
		TargetHumidityRange:    domain.Range{Min: 45, Max: 50},
		MaxTemperatureRate:     3,
		MaxHumidityRate:        12,
	}, application.Mutation{ExpectedVersion: created.Batch.Version, IdempotencyKey: "profile-" + key})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := service.GeneratePlan(ctx, profiled.Batch.ID, application.Mutation{ExpectedVersion: profiled.Batch.Version, IdempotencyKey: "plan-" + key})
	if err != nil {
		t.Fatal(err)
	}
	return planned.Batch
}
