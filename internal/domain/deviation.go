package domain

import "time"

type DeviationKind string

const (
	DeviationOutOfRange DeviationKind = "out_of_range"
	DeviationRate       DeviationKind = "rate_exceeded"
	DeviationGap        DeviationKind = "sampling_gap"
)

type Deviation struct {
	ID                     string        `json:"id"`
	BatchID                string        `json:"batchId"`
	StageID                string        `json:"stageId"`
	ReadingID              string        `json:"readingId"`
	Kind                   DeviationKind `json:"kind"`
	Details                string        `json:"details"`
	CheckpointSequence     int           `json:"checkpointSequence"`
	CreatedAt              time.Time     `json:"createdAt"`
	ResolvedAt             *time.Time    `json:"resolvedAt,omitempty"`
	Resolution             string        `json:"resolution,omitempty"`
	ResponsiblePerson      string        `json:"responsiblePerson,omitempty"`
	EnvironmentRemediation string        `json:"environmentRemediation,omitempty"`
	Conclusion             string        `json:"conclusion,omitempty"`
	CheckpointStageID      string        `json:"checkpointStageId,omitempty"`
}

func (d Deviation) IsOpen() bool { return d.ResolvedAt == nil }

type DeviationResolutionEvidence struct {
	ResponsiblePerson      string `json:"responsiblePerson"`
	EnvironmentRemediation string `json:"environmentRemediation"`
	Conclusion             string `json:"conclusion"`
}

func (e DeviationResolutionEvidence) Validate() error {
	if err := ValidateName("responsiblePerson", e.ResponsiblePerson, 80); err != nil {
		return err
	}
	if err := ValidateName("environmentRemediation", e.EnvironmentRemediation, 500); err != nil {
		return err
	}
	return ValidateName("conclusion", e.Conclusion, 500)
}
