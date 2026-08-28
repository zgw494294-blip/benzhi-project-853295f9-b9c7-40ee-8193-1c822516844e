package attempt_window_cache_test

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

func TestCorrectionAttemptDoesNotReuseHistoricalWindow(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	batch := &domain.AcclimatizationBatch{
		ID: "batch-cache", VenueID: "gallery-a", OwnerName: "林文", Status: domain.BatchRunning,
		Version: 1, CreatedAt: base, UpdatedAt: base, CurrentStageID: "stage-2",
		Profiles: []domain.ObjectMaterialProfile{{
			ID: "object-1", BatchID: "batch-cache", CollectionCode: "OBJ-001",
			MaxTemperatureRate: 3, MaxHumidityRate: 12,
		}},
		Stages: []domain.AcclimatizationStage{
			{ID: "stage-1", BatchID: "batch-cache", Sequence: 1, Status: domain.StageCompleted, Attempt: 1, StartedAt: timePointer(base.Add(-time.Hour)), CompletedAt: timePointer(base)},
			{ID: "stage-2", BatchID: "batch-cache", Sequence: 2, Status: domain.StageActive, Attempt: 1, StartedAt: timePointer(base), TargetTemperatureRange: domain.Range{Min: 20, Max: 22}, TargetHumidityRange: domain.Range{Min: 45, Max: 50}, MinimumDuration: 15 * time.Minute, StabilityWindow: 10 * time.Minute},
		},
	}
	if _, _, err = repository.CreateBatch(ctx, "create-cache", "payload", batch); err != nil {
		t.Fatal(err)
	}

	sequence := 0
	newID := func(prefix string) string {
		sequence++
		return fmt.Sprintf("%s-%d", prefix, sequence)
	}
	evaluator := &policy.Evaluator{MaxGap: 15 * time.Minute}
	service := application.NewService(repository, policy.Planner{NewID: newID}, evaluator, fixedClock{now: base.Add(time.Hour)}, newID)
	version := int64(1)
	for index, offset := range []time.Duration{0, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute} {
		result, submitErr := service.SubmitReading(ctx, batch.ID, application.ReadingInput{StageID: "stage-2", ObservedAt: base.Add(offset), Temperature: 21, Humidity: 47}, application.Mutation{ExpectedVersion: version, IdempotencyKey: fmt.Sprintf("attempt-1-%d", index)})
		if submitErr != nil {
			t.Fatalf("第一次执行提交读数失败: %v", submitErr)
		}
		version = result.Batch.Version
	}

	review, err := service.SubmitReview(ctx, batch.ID, application.Mutation{ExpectedVersion: version, IdempotencyKey: "submit-review"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := service.DecideReview(ctx, batch.ID, application.ReviewDecisionInput{ReviewerName: "周宁", Decision: domain.ReviewReturned, Reason: "需要重新采集", RequiredStageID: "stage-2", RequiredAction: "重新完成稳定窗口"}, application.Mutation{ExpectedVersion: review.Batch.Version, IdempotencyKey: "return-review"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.CompleteCorrection(ctx, batch.ID, application.Mutation{ExpectedVersion: decision.Batch.Version, IdempotencyKey: "prepare-correction"})
	if err != nil {
		t.Fatal(err)
	}
	running, err := service.StartBatch(ctx, batch.ID, application.Mutation{ExpectedVersion: ready.Batch.Version, IdempotencyKey: "start-attempt-2"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.SubmitReading(ctx, batch.ID, application.ReadingInput{StageID: "stage-2", ObservedAt: base.Add(20 * time.Minute), Temperature: 21, Humidity: 47}, application.Mutation{ExpectedVersion: running.Batch.Version, IdempotencyKey: "attempt-2-only-reading"})
	if err != nil {
		t.Fatal(err)
	}
	stage := result.Batch.StageByID("stage-2")
	if stage == nil {
		t.Fatal("第二阶段不存在")
	}
	if stage.Status != domain.StageActive || result.Decision == nil || result.Decision.Stable {
		t.Fatalf("TestCorrectionAttemptDoesNotReuseHistoricalWindow: 新 attempt 只有一条读数却得到 status=%s stable=%v", stage.Status, result.Decision != nil && result.Decision.Stable)
	}
}

func timePointer(value time.Time) *time.Time { return &value }
