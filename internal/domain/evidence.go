package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type EvidenceProfile struct {
	CollectionCode  string      `json:"collectionCode"`
	Materials       []string    `json:"materials"`
	Sensitivity     Sensitivity `json:"sensitivity"`
	Temperature     Range       `json:"temperature"`
	Humidity        Range       `json:"humidity"`
	MaxTempRate     float64     `json:"maxTempRate"`
	MaxHumidityRate float64     `json:"maxHumidityRate"`
}

type EvidenceStage struct {
	Sequence        int    `json:"sequence"`
	Temperature     Range  `json:"temperature"`
	Humidity        Range  `json:"humidity"`
	DurationSeconds int64  `json:"durationSeconds"`
	WindowSeconds   int64  `json:"windowSeconds"`
	Attempt         int    `json:"attempt"`
	ReadingCount    int    `json:"readingCount"`
	CompletedAt     string `json:"completedAt"`
}

type EvidenceDeviation struct {
	Kind       DeviationKind `json:"kind"`
	Stage      int           `json:"stage"`
	Resolution string        `json:"resolution"`
	ResolvedAt string        `json:"resolvedAt"`
}

type Evidence struct {
	Schema       string              `json:"schema"`
	BatchID      string              `json:"batchId"`
	VenueID      string              `json:"venueId"`
	OwnerName    string              `json:"ownerName"`
	Profiles     []EvidenceProfile   `json:"profiles"`
	Stages       []EvidenceStage     `json:"stages"`
	Deviations   []EvidenceDeviation `json:"deviations"`
	ReviewerName string              `json:"reviewerName"`
	ReviewReason string              `json:"reviewReason"`
}

func (b *AcclimatizationBatch) EvidenceView() (Evidence, error) {
	if b.Status != BatchApproved && b.Status != BatchCertified {
		return Evidence{}, InvalidState("只有 approved 或 certified 批次可以生成证据")
	}
	e := Evidence{Schema: CredentialSchemaVersion, BatchID: b.ID, VenueID: b.VenueID, OwnerName: b.OwnerName, Profiles: make([]EvidenceProfile, 0, len(b.Profiles)), Stages: make([]EvidenceStage, 0, len(b.Stages)), Deviations: make([]EvidenceDeviation, 0, len(b.Deviations))}
	for _, p := range b.Profiles {
		e.Profiles = append(e.Profiles, EvidenceProfile{CollectionCode: p.CollectionCode, Materials: append([]string(nil), p.MaterialClasses...), Sensitivity: p.SensitivityLevel, Temperature: p.TargetTemperatureRange, Humidity: p.TargetHumidityRange, MaxTempRate: p.MaxTemperatureRate, MaxHumidityRate: p.MaxHumidityRate})
	}
	for _, s := range b.Stages {
		if s.CompletedAt == nil {
			return Evidence{}, Integrity("证据包含未完成阶段")
		}
		count := 0
		for _, r := range b.Readings {
			if r.StageID == s.ID && r.Attempt == s.Attempt {
				count++
			}
		}
		e.Stages = append(e.Stages, EvidenceStage{Sequence: s.Sequence, Temperature: s.TargetTemperatureRange, Humidity: s.TargetHumidityRange, DurationSeconds: int64(s.MinimumDuration.Seconds()), WindowSeconds: int64(s.StabilityWindow.Seconds()), Attempt: s.Attempt, ReadingCount: count, CompletedAt: s.CompletedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")})
	}
	for _, d := range b.Deviations {
		if d.ResolvedAt == nil {
			return Evidence{}, Integrity("证据包含未处置偏差")
		}
		stage := b.StageByID(d.StageID)
		seq := 0
		if stage != nil {
			seq = stage.Sequence
		}
		e.Deviations = append(e.Deviations, EvidenceDeviation{Kind: d.Kind, Stage: seq, Resolution: d.Resolution, ResolvedAt: d.ResolvedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")})
	}
	if len(b.Reviews) == 0 || b.Reviews[len(b.Reviews)-1].Decision != ReviewApproved {
		return Evidence{}, Integrity("证据缺少批准复核")
	}
	last := b.Reviews[len(b.Reviews)-1]
	e.ReviewerName, e.ReviewReason = last.ReviewerName, last.Reason
	return e, nil
}

func DigestEvidence(e Evidence) (string, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
