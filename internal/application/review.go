package application

import (
	"context"

	"collection-acclimatization-pass/internal/domain"
)

func (s *Service) SubmitReview(ctx context.Context, batchID string, mutation Mutation) (BatchResult, error) {
	return s.simpleMutation(ctx, batchID, "submit-review", mutation, func(batch *domain.AcclimatizationBatch) error { return batch.SubmitReview() })
}

type ReviewDecisionInput struct {
	ReviewerName    string                `json:"reviewerName"`
	Decision        domain.ReviewDecision `json:"decision"`
	Reason          string                `json:"reason"`
	RequiredStageID string                `json:"requiredStageId,omitempty"`
	RequiredAction  string                `json:"requiredAction,omitempty"`
}

func (s *Service) DecideReview(ctx context.Context, batchID string, input ReviewDecisionInput, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, "decide-review", mutation.IdempotencyKey, payloadHash(input), func(batch *domain.AcclimatizationBatch) error {
		now := s.clock.Now()
		record := domain.ReviewRecord{ID: s.newID("review"), BatchID: batchID, ReviewerName: input.ReviewerName, Decision: input.Decision, Reason: input.Reason, RequiredStageID: input.RequiredStageID, RequiredAction: input.RequiredAction, SubmittedAt: now, DecidedAt: now, SubmissionVersion: mutation.ExpectedVersion}
		return batch.DecideReview(record)
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}

func (s *Service) CompleteCorrection(ctx context.Context, batchID string, mutation Mutation) (BatchResult, error) {
	return s.simpleMutation(ctx, batchID, "complete-correction", mutation, func(batch *domain.AcclimatizationBatch) error {
		if batch.Status == domain.BatchCorrection {
			batch.Status = domain.BatchReady
			batch.CurrentStageID = batch.CorrectionStageID
			return nil
		}
		if len(batch.CorrectionTasks) == 0 {
			return domain.InvalidState("当前批次没有待完成的补正任务")
		}
		return batch.CompleteCorrection(s.clock.Now())
	})
}

type CorrectionProgress struct {
	Tasks    []domain.CorrectionTask `json:"tasks"`
	Complete bool                    `json:"complete"`
}

func (s *Service) GetCorrectionProgress(ctx context.Context, batchID string) (CorrectionProgress, error) {
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return CorrectionProgress{}, err
	}
	complete := len(batch.CorrectionTasks) > 0
	for _, task := range batch.CorrectionTasks {
		if task.CompletedAt == nil {
			complete = false
		}
	}
	return CorrectionProgress{Tasks: append([]domain.CorrectionTask(nil), batch.CorrectionTasks...), Complete: complete}, nil
}
