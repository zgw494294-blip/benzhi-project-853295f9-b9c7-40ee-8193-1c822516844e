package policy

import (
	"testing"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

func TestEvaluatorClassifiesGapAndRange(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := &domain.AcclimatizationBatch{Profiles: []domain.ObjectMaterialProfile{{MaxTemperatureRate: 2, MaxHumidityRate: 8}}}
	stage := domain.AcclimatizationStage{ID: "s1", Sequence: 1, Attempt: 1, TargetTemperatureRange: domain.Range{Min: 20, Max: 22}, TargetHumidityRange: domain.Range{Min: 45, Max: 50}, MinimumDuration: 15 * time.Minute, StabilityWindow: 10 * time.Minute}
	e := Evaluator{MaxGap: 15 * time.Minute}
	b.Readings = []domain.Reading{{StageID: "s1", Attempt: 1, ObservedAt: now, Temperature: 21, Humidity: 47}}
	result, err := e.Evaluate(b, stage, domain.Reading{ObservedAt: now.Add(20 * time.Minute), Temperature: 21, Humidity: 47})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeviationKind != domain.DeviationGap {
		t.Fatalf("期望采样缺口，得到 %s", result.DeviationKind)
	}
	b.Readings = nil
	result, err = e.Evaluate(b, stage, domain.Reading{ObservedAt: now.Add(time.Minute), Temperature: 25, Humidity: 47})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeviationKind != domain.DeviationOutOfRange {
		t.Fatalf("期望越界，得到 %s", result.DeviationKind)
	}
}
