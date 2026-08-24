package policy

import (
	"fmt"
	"math"
	"sort"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

type Evaluation struct {
	Verdict            string               `json:"verdict"`
	Stable             bool                 `json:"stable"`
	DeviationKind      domain.DeviationKind `json:"deviationKind,omitempty"`
	Details            string               `json:"details"`
	CheckpointSequence int                  `json:"checkpointSequence,omitempty"`
	StableSince        *time.Time           `json:"stableSince,omitempty"`
	TemperatureRate    float64              `json:"temperatureRate,omitempty"`
	HumidityRate       float64              `json:"humidityRate,omitempty"`
}

type Evaluator struct{ MaxGap time.Duration }

func (e Evaluator) Evaluate(batch *domain.AcclimatizationBatch, stage domain.AcclimatizationStage, candidate domain.Reading) (Evaluation, error) {
	if candidate.ObservedAt.IsZero() {
		return Evaluation{}, domain.Validation("observedAt", "必须提供采样时间")
	}
	if math.IsNaN(candidate.Temperature) || math.IsInf(candidate.Temperature, 0) {
		return Evaluation{}, domain.Validation("temperature", "必须是有限数值")
	}
	if math.IsNaN(candidate.Humidity) || math.IsInf(candidate.Humidity, 0) {
		return Evaluation{}, domain.Validation("humidity", "必须是有限数值")
	}
	readings := attemptReadings(batch.Readings, stage.ID, stage.Attempt)
	if len(readings) > 0 && !candidate.ObservedAt.After(readings[len(readings)-1].ObservedAt) {
		return Evaluation{}, domain.Validation("observedAt", "采样时间必须严格递增")
	}
	checkpoint := stage.Sequence
	if len(readings) > 0 {
		previous := readings[len(readings)-1]
		gap := candidate.ObservedAt.Sub(previous.ObservedAt)
		maxGap := e.MaxGap
		if maxGap <= 0 {
			maxGap = 15 * time.Minute
		}
		if gap > maxGap {
			return Evaluation{Verdict: "isolate", DeviationKind: domain.DeviationGap, Details: fmt.Sprintf("采样间隔 %s 超过允许值 %s", gap, maxGap), CheckpointSequence: checkpoint}, nil
		}
		hours := gap.Hours()
		tempRate := math.Abs(candidate.Temperature-previous.Temperature) / hours
		humidityRate := math.Abs(candidate.Humidity-previous.Humidity) / hours
		maxTemp, maxHumidity := strictRates(batch.Profiles)
		if tempRate > maxTemp || humidityRate > maxHumidity {
			return Evaluation{Verdict: "isolate", DeviationKind: domain.DeviationRate, Details: fmt.Sprintf("变化率 %.2f °C/h、%.2f %%RH/h 超过阈值 %.2f、%.2f", tempRate, humidityRate, maxTemp, maxHumidity), CheckpointSequence: checkpoint, TemperatureRate: tempRate, HumidityRate: humidityRate}, nil
		}
	}
	if !stage.TargetTemperatureRange.Contains(candidate.Temperature) || !stage.TargetHumidityRange.Contains(candidate.Humidity) {
		return Evaluation{Verdict: "isolate", DeviationKind: domain.DeviationOutOfRange, Details: "读数超出当前阶段目标温湿度区间", CheckpointSequence: checkpoint}, nil
	}
	readings = append(readings, candidate)
	first := readings[0].ObservedAt
	if candidate.ObservedAt.Sub(first) < stage.MinimumDuration {
		return Evaluation{Verdict: "collecting", Details: "尚未达到阶段最短持续时间"}, nil
	}
	windowStart := candidate.ObservedAt.Add(-stage.StabilityWindow)
	var stableSince *time.Time
	for _, reading := range readings {
		if !reading.ObservedAt.Before(windowStart) {
			t := reading.ObservedAt
			stableSince = &t
			break
		}
	}
	if stableSince == nil || candidate.ObservedAt.Sub(*stableSince) < stage.StabilityWindow {
		return Evaluation{Verdict: "collecting", Details: "连续稳定窗口采样覆盖不足"}, nil
	}
	return Evaluation{Verdict: "stage_completed", Stable: true, StableSince: stableSince, Details: "最短持续时间和连续稳定窗口均已满足"}, nil
}

func attemptReadings(all []domain.Reading, stageID string, attempt int) []domain.Reading {
	result := make([]domain.Reading, 0)
	for _, reading := range all {
		if reading.StageID == stageID && reading.Attempt == attempt {
			result = append(result, reading)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ObservedAt.Before(result[j].ObservedAt) })
	return result
}

func strictRates(profiles []domain.ObjectMaterialProfile) (float64, float64) {
	temp, humidity := math.MaxFloat64, math.MaxFloat64
	for _, profile := range profiles {
		temp = math.Min(temp, profile.MaxTemperatureRate)
		humidity = math.Min(humidity, profile.MaxHumidityRate)
	}
	return temp, humidity
}
