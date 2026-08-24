package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/policy"
)

type ReadingInput struct {
	StageID     string    `json:"stageId"`
	ObservedAt  time.Time `json:"observedAt"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
}

func (s *Service) SubmitReading(ctx context.Context, batchID string, input ReadingInput, mutation Mutation) (BatchResult, error) {
	return s.SubmitReadings(ctx, batchID, []ReadingInput{input}, mutation)
}

type ReadingItemResult struct {
	Index                       int    `json:"index"`
	ReadingID                   string `json:"readingId,omitempty"`
	Verdict                     string `json:"verdict"`
	Details                     string `json:"details,omitempty"`
	StableWindowCoverageSeconds int64  `json:"stableWindowCoverageSeconds"`
}

func (s *Service) SubmitReadings(ctx context.Context, batchID string, inputs []ReadingInput, mutation Mutation) (BatchResult, error) {
	if err := validateMutation(mutation); err != nil {
		return BatchResult{}, domain.Validation("mutation", err.Error())
	}
	if len(inputs) == 0 || len(inputs) > 500 {
		return BatchResult{}, domain.Validation("readings", "读数条数必须在 1 到 500 之间")
	}
	var results []ReadingItemResult
	var evaluation policy.Evaluation
	receipt, replayed, err := s.store.UpdateBatchWithResult(ctx, batchID, mutation.ExpectedVersion, "submit-readings", mutation.IdempotencyKey, payloadHash(inputs), func(batch *domain.AcclimatizationBatch) (json.RawMessage, error) {
		results = make([]ReadingItemResult, 0, len(inputs))
		for index, input := range inputs {
			if batch.Status != domain.BatchRunning {
				return nil, domain.InvalidState("只有 running 批次可以提交读数")
			}
			if input.StageID != batch.CurrentStageID {
				return nil, domain.Validation(fmt.Sprintf("readings[%d].stageId", index), "读数必须属于当前阶段")
			}
			stage := batch.StageByID(input.StageID)
			if stage == nil {
				return nil, domain.NotFound("阶段", input.StageID)
			}
			reading := domain.Reading{ID: s.newID("reading"), BatchID: batchID, StageID: input.StageID, Attempt: stage.Attempt, ObservedAt: input.ObservedAt.UTC(), Temperature: input.Temperature, Humidity: input.Humidity}
			var err error
			evaluation, err = s.evaluator.Evaluate(batch, *stage, reading)
			if err != nil {
				if validation, ok := err.(*domain.Error); ok && validation.Field == "observedAt" {
					evaluation = policy.Evaluation{Verdict: "isolate", DeviationKind: domain.DeviationGap, Details: validation.Message, CheckpointSequence: stage.Sequence}
					results = append(results, ReadingItemResult{Index: index, Verdict: evaluation.Verdict, Details: evaluation.Details})
					deviation, deviationErr := policy.NewDeviation(s.newID, batchID, stage.ID, "", evaluation, s.clock.Now())
					if deviationErr != nil {
						return nil, deviationErr
					}
					batch.Deviations = append(batch.Deviations, deviation)
					if isolateErr := batch.Isolate(); isolateErr != nil {
						return nil, isolateErr
					}
					break
				}
				return nil, err
			}
			reading.Verdict = evaluation.Verdict
			batch.Readings = append(batch.Readings, reading)
			item := ReadingItemResult{Index: index, ReadingID: reading.ID, Verdict: evaluation.Verdict, Details: evaluation.Details}
			if evaluation.StableSince != nil {
				item.StableWindowCoverageSeconds = int64(input.ObservedAt.Sub(*evaluation.StableSince).Seconds())
			}
			results = append(results, item)
			if evaluation.Verdict == "isolate" {
				deviation, err := policy.NewDeviation(s.newID, batchID, stage.ID, reading.ID, evaluation, s.clock.Now())
				if err != nil {
					return nil, err
				}
				batch.Deviations = append(batch.Deviations, deviation)
				if err := batch.Isolate(); err != nil {
					return nil, err
				}
				break
			}
			if evaluation.Stable {
				if err := batch.CompleteCurrentStage(input.ObservedAt); err != nil {
					return nil, err
				}
				if batch.Status == domain.BatchRunning && batch.StageByID(batch.CurrentStageID) != nil {
					_ = batch.StageByID(batch.CurrentStageID)
				}
			}
		}
		encoded, _ := json.Marshal(results)
		return encoded, nil
	})
	if err != nil {
		return BatchResult{}, err
	}
	if replayed {
		evaluation = policy.Evaluation{Verdict: "replayed", Details: "返回幂等请求的原始批次快照"}
		_ = json.Unmarshal(receipt.Result, &results)
	}
	return BatchResult{Batch: receipt.Batch, Replayed: replayed, Decision: &evaluation, ReadingResults: results, CurrentStageID: receipt.Batch.CurrentStageID}, nil
}

type ReadingQuery struct {
	StageID string
	Attempt int
	From    *time.Time
	To      *time.Time
	Verdict string
	Cursor  string
	Limit   int
}
type ReadingStats struct {
	TemperatureMin           float64 `json:"temperatureMin"`
	TemperatureMax           float64 `json:"temperatureMax"`
	TemperatureAverage       float64 `json:"temperatureAverage"`
	HumidityMin              float64 `json:"humidityMin"`
	HumidityMax              float64 `json:"humidityMax"`
	HumidityAverage          float64 `json:"humidityAverage"`
	SampleCount              int     `json:"sampleCount"`
	TimeSpanSeconds          int64   `json:"timeSpanSeconds"`
	StabilityCoverageSeconds int64   `json:"stabilityCoverageSeconds"`
	StabilityCoverage        float64 `json:"stabilityCoverage"`
}
type ReadingQueryResult struct {
	Items          []domain.Reading `json:"items"`
	NextCursor     string           `json:"nextCursor,omitempty"`
	Stats          ReadingStats     `json:"stats"`
	StageCompleted bool             `json:"stageCompleted"`
	HasIsolation   bool             `json:"hasIsolation"`
	Gaps           []string         `json:"gaps"`
}

func (s *Service) QueryReadings(ctx context.Context, batchID string, query ReadingQuery) (ReadingQueryResult, error) {
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return ReadingQueryResult{}, err
	}
	if query.StageID != "" && batch.StageByID(query.StageID) == nil {
		return ReadingQueryResult{}, domain.NotFound("阶段", query.StageID)
	}
	if query.Verdict != "" && query.Verdict != "collecting" && query.Verdict != "stage_completed" && query.Verdict != "isolate" {
		return ReadingQueryResult{}, domain.Validation("verdict", "未知读数判定")
	}
	if query.From != nil && query.To != nil && query.To.Before(*query.From) {
		return ReadingQueryResult{}, domain.Validation("to", "结束时间不能早于开始时间")
	}
	if query.Limit == 0 {
		query.Limit = 100
	}
	if query.Limit < 1 || query.Limit > 500 {
		return ReadingQueryResult{}, domain.Validation("limit", "分页条数必须在 1 到 500 之间")
	}
	items := make([]domain.Reading, 0)
	for _, reading := range batch.Readings {
		if query.StageID != "" && reading.StageID != query.StageID {
			continue
		}
		if query.Attempt > 0 && reading.Attempt != query.Attempt {
			continue
		}
		if query.Verdict != "" && reading.Verdict != query.Verdict {
			continue
		}
		if query.From != nil && reading.ObservedAt.Before(*query.From) {
			continue
		}
		if query.To != nil && reading.ObservedAt.After(*query.To) {
			continue
		}
		items = append(items, reading)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ObservedAt.Equal(items[j].ObservedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].ObservedAt.Before(items[j].ObservedAt)
	})
	start := 0
	if query.Cursor != "" {
		for i, item := range items {
			if item.ID == query.Cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + query.Limit
	if end > len(items) {
		end = len(items)
	}
	result := ReadingQueryResult{Items: items[start:end], Gaps: []string{}}
	if end < len(items) && end > start {
		result.NextCursor = items[end-1].ID
	}
	for i, item := range result.Items {
		if item.Verdict == "isolate" {
			result.HasIsolation = true
		}
		if item.Verdict == "sampling_gap" {
			result.Gaps = append(result.Gaps, item.ObservedAt.Format(time.RFC3339))
		}
		if i > 0 && item.ObservedAt.Sub(result.Items[i-1].ObservedAt) > 15*time.Minute {
			result.Gaps = append(result.Gaps, result.Items[i-1].ObservedAt.Format(time.RFC3339)+"/"+item.ObservedAt.Format(time.RFC3339))
		}
	}
	if query.StageID != "" {
		stage := batch.StageByID(query.StageID)
		result.StageCompleted = stage.Status == domain.StageCompleted
		if len(result.Items) > 0 {
			result.Stats = statsForReadings(result.Items, stage.StabilityWindow)
		}
	}
	return result, nil
}

func statsForReadings(readings []domain.Reading, window time.Duration) ReadingStats {
	result := ReadingStats{TemperatureMin: readings[0].Temperature, TemperatureMax: readings[0].Temperature, HumidityMin: readings[0].Humidity, HumidityMax: readings[0].Humidity, SampleCount: len(readings)}
	var tempSum, humSum float64
	first, last := readings[0].ObservedAt, readings[0].ObservedAt
	for _, r := range readings {
		if r.Temperature < result.TemperatureMin {
			result.TemperatureMin = r.Temperature
		}
		if r.Temperature > result.TemperatureMax {
			result.TemperatureMax = r.Temperature
		}
		if r.Humidity < result.HumidityMin {
			result.HumidityMin = r.Humidity
		}
		if r.Humidity > result.HumidityMax {
			result.HumidityMax = r.Humidity
		}
		tempSum += r.Temperature
		humSum += r.Humidity
		if r.ObservedAt.Before(first) {
			first = r.ObservedAt
		}
		if r.ObservedAt.After(last) {
			last = r.ObservedAt
		}
	}
	result.TemperatureAverage = tempSum / float64(len(readings))
	result.HumidityAverage = humSum / float64(len(readings))
	result.TimeSpanSeconds = int64(last.Sub(first).Seconds())
	if result.TimeSpanSeconds > 0 && window > 0 {
		result.StabilityCoverageSeconds = result.TimeSpanSeconds
		result.StabilityCoverage = float64(result.TimeSpanSeconds) / window.Seconds()
		if result.StabilityCoverage > 1 {
			result.StabilityCoverage = 1
		}
	}
	return result
}
