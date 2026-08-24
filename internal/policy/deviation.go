package policy

import (
	"time"

	"collection-acclimatization-pass/internal/domain"
)

func NewDeviation(newID IDGenerator, batchID, stageID, readingID string, evaluation Evaluation, now time.Time) (domain.Deviation, error) {
	if evaluation.DeviationKind == "" || evaluation.Verdict != "isolate" {
		return domain.Deviation{}, domain.Validation("evaluation", "只有隔离判定可以生成偏差")
	}
	if evaluation.CheckpointSequence < 1 {
		return domain.Deviation{}, domain.Integrity("偏差缺少重跑检查点")
	}
	return domain.Deviation{ID: newID("deviation"), BatchID: batchID, StageID: stageID, ReadingID: readingID, Kind: evaluation.DeviationKind, Details: evaluation.Details, CheckpointSequence: evaluation.CheckpointSequence, CreatedAt: now.UTC()}, nil
}
