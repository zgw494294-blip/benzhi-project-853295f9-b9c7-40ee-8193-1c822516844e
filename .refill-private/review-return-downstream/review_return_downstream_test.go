package review_return_downstream_test

import (
	"context"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestReturnedReviewResetsEveryDownstreamStage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC)
	repository, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("打开仓储: %v", err)
	}

	stages := make([]domain.AcclimatizationStage, 3)
	for i := range stages {
		startedAt := now.Add(time.Duration(i-6) * time.Hour)
		completedAt := startedAt.Add(time.Hour)
		stages[i] = domain.AcclimatizationStage{
			ID: "stage-" + string(rune('1'+i)), BatchID: "batch-review-return", Sequence: i + 1,
			Status: domain.StageCompleted, Attempt: 1, StartedAt: &startedAt, CompletedAt: &completedAt,
		}
	}
	batch := &domain.AcclimatizationBatch{
		ID: "batch-review-return", VenueID: "gallery-a", OwnerName: "林文",
		Status: domain.BatchReview, CurrentStageID: "stage-3", Version: 1,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now, Stages: stages,
	}
	if _, _, err = repository.CreateBatch(ctx, "setup-review-return", "setup-payload", batch); err != nil {
		t.Fatalf("写入测试批次: %v", err)
	}

	service := application.NewService(repository, nil, nil, fixedClock{now: now}, func(string) string { return "review-return-1" })
	_, err = service.DecideReview(ctx, batch.ID, application.ReviewDecisionInput{
		ReviewerName: "周宁", Decision: domain.ReviewReturned, Reason: "第一阶段稳定窗口证据不足",
		RequiredStageID: "stage-1", RequiredAction: "从第一阶段重新采集并顺序完成全部后续阶段",
	}, application.Mutation{ExpectedVersion: 1, IdempotencyKey: "return-review"})
	if err != nil {
		t.Fatalf("提交退回复核决定: %v", err)
	}

	persisted, err := repository.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("读取退回后的批次: %v", err)
	}
	for _, stage := range persisted.Stages {
		if stage.Status != domain.StagePlanned || stage.StartedAt != nil || stage.CompletedAt != nil {
			t.Fatalf("阶段 %d 仍复用退回前的完成状态: status=%s startedAt=%v completedAt=%v", stage.Sequence, stage.Status, stage.StartedAt, stage.CompletedAt)
		}
	}
}
