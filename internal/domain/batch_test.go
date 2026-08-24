package domain

import (
	"testing"
	"time"
)

func testBatch(t *testing.T) *AcclimatizationBatch {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b, err := NewBatch("batch-1", "venue", "保护员", now.Add(time.Hour), Range{Min: 20, Max: 22}, Range{Min: 45, Max: 50}, now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProfile(ObjectMaterialProfile{ID: "obj-1", BatchID: b.ID, CollectionCode: "OBJ-1", MaterialClasses: []string{"纸"}, SensitivityLevel: SensitivityMedium, TargetTemperatureRange: Range{Min: 20, Max: 22}, TargetHumidityRange: Range{Min: 45, Max: 50}, MaxTemperatureRate: 3, MaxHumidityRate: 12}, b.VenueTemperatureRange, b.VenueHumidityRange)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.AddProfile(p); err != nil {
		t.Fatal(err)
	}
	b.SetPlan([]AcclimatizationStage{{ID: "s1", BatchID: b.ID, Sequence: 1, TargetTemperatureRange: Range{Min: 20, Max: 22}, TargetHumidityRange: Range{Min: 45, Max: 50}, MinimumDuration: 15 * time.Minute, StabilityWindow: 10 * time.Minute, Attempt: 1}, {ID: "s2", BatchID: b.ID, Sequence: 2, TargetTemperatureRange: Range{Min: 20, Max: 22}, TargetHumidityRange: Range{Min: 45, Max: 50}, MinimumDuration: 15 * time.Minute, StabilityWindow: 10 * time.Minute, Attempt: 1}})
	return b
}

func TestBatchStateTransitions(t *testing.T) {
	b := testBatch(t)
	if err := b.Start(time.Now()); err == nil {
		t.Fatal("draft 不应直接启动")
	}
	if err := b.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := b.SubmitReview(); err == nil {
		t.Fatal("未完成阶段不应提交复核")
	}
}

func TestEvidenceDigestDeterministic(t *testing.T) {
	b := testBatch(t)
	_ = b.Freeze()
	_ = b.Start(time.Now())
	now := time.Now().UTC()
	for i := range b.Stages {
		b.Stages[i].Status = StageCompleted
		b.Stages[i].CompletedAt = &now
	}
	b.Status = BatchApproved
	b.Reviews = append(b.Reviews, ReviewRecord{ReviewerName: "复核员", Decision: ReviewApproved, Reason: "通过"})
	evidence, err := b.EvidenceView()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := DigestEvidence(evidence)
	second, _ := DigestEvidence(evidence)
	if first != second {
		t.Fatal("摘要必须确定性")
	}
}
