package credentialreplaybypass_test

import (
	"context"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

func TestCertifiedBatchRejectsDifferentIssuerRequest(t *testing.T) {
	repository, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	completed := now.Add(time.Hour)
	batch := &domain.AcclimatizationBatch{
		ID: "batch-approved", VenueID: "venue-a", OwnerName: "保管员",
		PlannedStartAt: now, Status: domain.BatchApproved, Version: 1,
		CreatedAt: now, UpdatedAt: now,
		Profiles: []domain.ObjectMaterialProfile{{
			ID: "object-1", BatchID: "batch-approved", CollectionCode: "OBJ-1",
			MaterialClasses: []string{"纸"}, SensitivityLevel: domain.SensitivityMedium,
			TargetTemperatureRange: domain.Range{Min: 20, Max: 22},
			TargetHumidityRange:    domain.Range{Min: 45, Max: 50},
			MaxTemperatureRate:     3, MaxHumidityRate: 12,
		}},
		Stages: []domain.AcclimatizationStage{{
			ID: "stage-1", BatchID: "batch-approved", Sequence: 1,
			TargetTemperatureRange: domain.Range{Min: 20, Max: 22},
			TargetHumidityRange:    domain.Range{Min: 45, Max: 50},
			MinimumDuration:        15 * time.Minute, StabilityWindow: 10 * time.Minute,
			Status: domain.StageCompleted, Attempt: 1, CompletedAt: &completed,
		}},
		Reviews: []domain.ReviewRecord{{ReviewerName: "原复核员", Decision: domain.ReviewApproved, Reason: "证据完整"}},
	}
	if _, _, err := repository.CreateBatch(context.Background(), "seed", "seed", batch); err != nil {
		t.Fatal(err)
	}
	sequence := 0
	service := application.NewService(repository, nil, nil, nil, func(prefix string) string {
		sequence++
		return prefix + "-" + string(rune('0'+sequence))
	})
	issued, err := service.IssueCredential(context.Background(), batch.ID, "原复核员", application.Mutation{ExpectedVersion: 1, IdempotencyKey: "issue-original"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.IssueCredential(context.Background(), batch.ID, "冒名复核员", application.Mutation{ExpectedVersion: issued.Batch.Version, IdempotencyKey: "issue-different"})
	if err == nil {
		t.Fatal("已认证批次把不同签发人和不同幂等键误判为原请求重放")
	}
}
