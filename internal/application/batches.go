package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

type CreateBatchInput struct {
	VenueID               string       `json:"venueId"`
	PlannedStartAt        time.Time    `json:"plannedStartAt"`
	OwnerName             string       `json:"ownerName"`
	VenueTemperatureRange domain.Range `json:"venueTemperatureRange"`
	VenueHumidityRange    domain.Range `json:"venueHumidityRange"`
}

func (s *Service) CreateBatch(ctx context.Context, input CreateBatchInput, idempotencyKey string) (BatchResult, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return BatchResult{}, domain.Validation("Idempotency-Key", "创建批次必须提供幂等请求键")
	}
	now := s.clock.Now()
	batch, err := domain.NewBatch(s.newID("batch"), input.VenueID, input.OwnerName, input.PlannedStartAt, input.VenueTemperatureRange, input.VenueHumidityRange, now)
	if err != nil {
		return BatchResult{}, err
	}
	receipt, replayed, err := s.store.CreateBatch(ctx, idempotencyKey, payloadHash(input), batch)
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}

func (s *Service) GetBatch(ctx context.Context, id string) (*domain.AcclimatizationBatch, error) {
	return s.store.GetBatch(ctx, id)
}

type AddProfileInput struct {
	CollectionCode         string             `json:"collectionCode"`
	MaterialClasses        []string           `json:"materialClasses"`
	SensitivityLevel       domain.Sensitivity `json:"sensitivityLevel"`
	TargetTemperatureRange domain.Range       `json:"targetTemperatureRange"`
	TargetHumidityRange    domain.Range       `json:"targetHumidityRange"`
	MaxTemperatureRate     float64            `json:"maxTemperatureRate"`
	MaxHumidityRate        float64            `json:"maxHumidityRate"`
}

func (s *Service) AddProfile(ctx context.Context, batchID string, input AddProfileInput, mutation Mutation) (BatchResult, error) {
	return s.AddProfiles(ctx, batchID, []AddProfileInput{input}, mutation)
}

func (s *Service) AddProfiles(ctx context.Context, batchID string, inputs []AddProfileInput, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	if len(inputs) == 0 || len(inputs) > 100 {
		return BatchResult{}, domain.Validation("profiles", "材料档案条数必须在 1 到 100 之间")
	}
	receipt, replayed, err := s.store.UpdateBatch(ctx, batchID, mutation.ExpectedVersion, "add-profiles", mutation.IdempotencyKey, payloadHash(inputs), func(batch *domain.AcclimatizationBatch) error {
		profiles := make([]domain.ObjectMaterialProfile, 0, len(inputs))
		for i, input := range inputs {
			profile, err := domain.NewProfile(domain.ObjectMaterialProfile{ID: s.newID("object"), BatchID: batchID, CollectionCode: input.CollectionCode, MaterialClasses: input.MaterialClasses, SensitivityLevel: input.SensitivityLevel, TargetTemperatureRange: input.TargetTemperatureRange, TargetHumidityRange: input.TargetHumidityRange, MaxTemperatureRate: input.MaxTemperatureRate, MaxHumidityRate: input.MaxHumidityRate}, batch.VenueTemperatureRange, batch.VenueHumidityRange)
			if err != nil {
				if validation, ok := err.(*domain.Error); ok {
					validation.Field = fmt.Sprintf("profiles[%d].%s", i, validation.Field)
				}
				return err
			}
			profiles = append(profiles, profile)
		}
		return batch.AddProfiles(profiles)
	})
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed}, nil
}
