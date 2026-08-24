package application

import (
	"context"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

func (s *Service) ListDeviations(ctx context.Context, batchID string) ([]domain.Deviation, error) {
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	return append([]domain.Deviation(nil), batch.Deviations...), nil
}

type ResolveDeviationInput struct {
	Resolution             string `json:"resolution"`
	ResponsiblePerson      string `json:"responsiblePerson"`
	EnvironmentRemediation string `json:"environmentRemediation"`
	Conclusion             string `json:"conclusion"`
	CheckpointSequence     int    `json:"checkpointSequence"`
	MinimumDurationMinutes *int   `json:"minimumDurationMinutes,omitempty"`
	StabilityWindowMinutes *int   `json:"stabilityWindowMinutes,omitempty"`
}

func (s *Service) ResolveDeviation(ctx context.Context, batchID, deviationID string, input ResolveDeviationInput, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	payload := struct {
		ID    string
		Input ResolveDeviationInput
	}{deviationID, input}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, "resolve-deviation:"+deviationID, mutation.IdempotencyKey, payloadHash(payload), func(batch *domain.AcclimatizationBatch) error {
		if input.MinimumDurationMinutes != nil || input.StabilityWindowMinutes != nil {
			stage := batch.StageByID(batch.CurrentStageID)
			if stage == nil {
				return domain.Integrity("隔离批次缺少当前阶段")
			}
			duration, window := stage.MinimumDuration, stage.StabilityWindow
			if input.MinimumDurationMinutes != nil {
				duration = time.Duration(*input.MinimumDurationMinutes) * time.Minute
			}
			if input.StabilityWindowMinutes != nil {
				window = time.Duration(*input.StabilityWindowMinutes) * time.Minute
			}
			if err := batch.ReviseStage(stage.ID, duration, window); err != nil {
				return err
			}
		}
		conclusion := input.Conclusion
		if conclusion == "" {
			conclusion = input.Resolution
		}
		return batch.ResolveDeviation(deviationID, domain.DeviationResolutionEvidence{ResponsiblePerson: input.ResponsiblePerson, EnvironmentRemediation: input.EnvironmentRemediation, Conclusion: conclusion}, input.CheckpointSequence, s.clock.Now())
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}
