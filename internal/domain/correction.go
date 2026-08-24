package domain

import "time"

type CorrectionTask struct {
	ID                string     `json:"id"`
	RequiredStageID   string     `json:"requiredStageId"`
	RequiredAction    string     `json:"requiredAction"`
	SubmissionVersion int64      `json:"submissionVersion"`
	RequiredAttempt   int        `json:"requiredAttempt"`
	CreatedAt         time.Time  `json:"createdAt"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
}

func (t CorrectionTask) ValidateCompletion(batch *AcclimatizationBatch) error {
	stage := batch.StageByID(t.RequiredStageID)
	if stage == nil {
		return Integrity("补正任务指定的阶段不存在")
	}
	if stage.Attempt < t.RequiredAttempt || stage.StartedAt == nil || stage.CompletedAt == nil || stage.Status != StageCompleted {
		return InvalidState("指定阶段尚未完成要求的重跑")
	}
	var first, last *time.Time
	for i := range batch.Readings {
		reading := &batch.Readings[i]
		if reading.StageID != stage.ID || reading.Attempt != stage.Attempt || reading.Verdict == "isolate" {
			continue
		}
		if first == nil || reading.ObservedAt.Before(*first) {
			value := reading.ObservedAt
			first = &value
		}
		if last == nil || reading.ObservedAt.After(*last) {
			value := reading.ObservedAt
			last = &value
		}
	}
	if first == nil || last == nil || last.Sub(*first) < stage.MinimumDuration {
		return InvalidState("指定阶段缺少最短持续时间和稳定窗口证据")
	}
	return nil
}
