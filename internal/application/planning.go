package application

import (
	"context"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

func (s *Service) GeneratePlan(ctx context.Context, batchID string, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, "generate-plan", mutation.IdempotencyKey, payloadHash(struct{}{}), func(batch *domain.AcclimatizationBatch) error {
		stages, basis, err := s.planner.Generate(batch)
		if err != nil {
			return err
		}
		return batch.SetPlanWithBasis(stages, basis)
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}

type ReviseStageInput struct {
	MinimumDurationMinutes int `json:"minimumDurationMinutes"`
	StabilityWindowMinutes int `json:"stabilityWindowMinutes"`
}

func (s *Service) ReviseStage(ctx context.Context, batchID, stageID string, input ReviseStageInput, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	payload := struct {
		StageID string
		Input   ReviseStageInput
	}{stageID, input}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, "revise-stage:"+stageID, mutation.IdempotencyKey, payloadHash(payload), func(batch *domain.AcclimatizationBatch) error {
		return batch.ReviseStage(stageID, time.Duration(input.MinimumDurationMinutes)*time.Minute, time.Duration(input.StabilityWindowMinutes)*time.Minute)
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}

type PlanStageDiff struct {
	Sequence int                      `json:"sequence"`
	Old      domain.PlanStageSnapshot `json:"old"`
	New      domain.PlanStageSnapshot `json:"new"`
	Changed  bool                     `json:"changed"`
}
type PlanView struct {
	PlanDigest string           `json:"planDigest"`
	Basis      domain.PlanBasis `json:"basis"`
	Stages     []PlanStageDiff  `json:"stages"`
}

func (s *Service) GetPlan(ctx context.Context, batchID string) (PlanView, error) {
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return PlanView{}, err
	}
	if len(batch.Stages) == 0 {
		return PlanView{}, domain.NotFound("方案", batchID)
	}
	view := PlanView{PlanDigest: batch.PlanDigest, Basis: batch.PlanBasis, Stages: []PlanStageDiff{}}
	for i, stage := range batch.Stages {
		old := domain.PlanStageSnapshot{}
		if i < len(batch.PlanBaseline) {
			old = batch.PlanBaseline[i]
		}
		newValue := domain.SnapshotPlan([]domain.AcclimatizationStage{stage})[0]
		view.Stages = append(view.Stages, PlanStageDiff{Sequence: stage.Sequence, Old: old, New: newValue, Changed: old != newValue})
	}
	return view, nil
}

func (s *Service) FreezePlan(ctx context.Context, batchID, digest string, mutation Mutation) (BatchResult, error) {
	return s.simpleMutation(ctx, batchID, "freeze-plan", mutation, func(batch *domain.AcclimatizationBatch) error { return batch.FreezeWithDigest(digest) })
}

func (s *Service) StartBatch(ctx context.Context, batchID string, mutation Mutation) (BatchResult, error) {
	return s.simpleMutation(ctx, batchID, "start-batch", mutation, func(batch *domain.AcclimatizationBatch) error { return batch.Start(s.clock.Now()) })
}

func (s *Service) simpleMutation(ctx context.Context, batchID, operation string, mutation Mutation, fn func(*domain.AcclimatizationBatch) error) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, operation, mutation.IdempotencyKey, payloadHash(struct{}{}), fn)
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}
