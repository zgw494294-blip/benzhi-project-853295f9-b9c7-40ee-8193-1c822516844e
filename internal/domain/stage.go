package domain

import "time"

type StageStatus string

const (
	StagePlanned   StageStatus = "planned"
	StageActive    StageStatus = "active"
	StageCompleted StageStatus = "completed"
)

type AcclimatizationStage struct {
	ID                     string        `json:"id"`
	BatchID                string        `json:"batchId"`
	Sequence               int           `json:"sequence"`
	TargetTemperatureRange Range         `json:"targetTemperatureRange"`
	TargetHumidityRange    Range         `json:"targetHumidityRange"`
	MinimumDuration        time.Duration `json:"minimumDuration"`
	StabilityWindow        time.Duration `json:"stabilityWindow"`
	Rationale              string        `json:"rationale"`
	Status                 StageStatus   `json:"status"`
	Attempt                int           `json:"attempt"`
	StartedAt              *time.Time    `json:"startedAt,omitempty"`
	CompletedAt            *time.Time    `json:"completedAt,omitempty"`
}

func (s AcclimatizationStage) Validate() error {
	if s.Sequence < 1 {
		return Validation("sequence", "阶段序号必须从 1 开始")
	}
	if err := s.TargetTemperatureRange.Validate("targetTemperatureRange", -10, 40); err != nil {
		return err
	}
	if err := s.TargetHumidityRange.Validate("targetHumidityRange", 10, 80); err != nil {
		return err
	}
	if s.MinimumDuration < 15*time.Minute || s.MinimumDuration > 14*24*time.Hour {
		return Validation("minimumDuration", "最短持续时间必须在 15 分钟到 14 天之间")
	}
	if s.StabilityWindow < 10*time.Minute || s.StabilityWindow > s.MinimumDuration {
		return Validation("stabilityWindow", "稳定窗口必须至少 10 分钟且不超过阶段持续时间")
	}
	if s.Attempt < 1 {
		return Validation("attempt", "阶段尝试次数必须从 1 开始")
	}
	return nil
}

type Reading struct {
	ID          string    `json:"id"`
	BatchID     string    `json:"batchId"`
	StageID     string    `json:"stageId"`
	Attempt     int       `json:"attempt"`
	ObservedAt  time.Time `json:"observedAt"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	Verdict     string    `json:"verdict"`
}
