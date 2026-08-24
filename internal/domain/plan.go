package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type PlanBasis struct {
	Sensitivity        Sensitivity `json:"sensitivity"`
	MaxTemperatureRate float64     `json:"maxTemperatureRate"`
	MaxHumidityRate    float64     `json:"maxHumidityRate"`
}

type PlanStageSnapshot struct {
	Sequence               int           `json:"sequence"`
	TargetTemperatureRange Range         `json:"targetTemperatureRange"`
	TargetHumidityRange    Range         `json:"targetHumidityRange"`
	MinimumDuration        time.Duration `json:"minimumDuration"`
	StabilityWindow        time.Duration `json:"stabilityWindow"`
	Attempt                int           `json:"attempt"`
}

func SnapshotPlan(stages []AcclimatizationStage) []PlanStageSnapshot {
	result := make([]PlanStageSnapshot, len(stages))
	for i, stage := range stages {
		result[i] = PlanStageSnapshot{Sequence: stage.Sequence, TargetTemperatureRange: stage.TargetTemperatureRange, TargetHumidityRange: stage.TargetHumidityRange, MinimumDuration: stage.MinimumDuration, StabilityWindow: stage.StabilityWindow, Attempt: stage.Attempt}
	}
	return result
}

func (b *AcclimatizationBatch) CalculatePlanDigest() (string, error) {
	profiles := append([]ObjectMaterialProfile(nil), b.Profiles...)
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].CollectionCode < profiles[j].CollectionCode })
	type profileBasis struct {
		CollectionCode     string      `json:"collectionCode"`
		Sensitivity        Sensitivity `json:"sensitivity"`
		Temperature        Range       `json:"temperature"`
		Humidity           Range       `json:"humidity"`
		MaxTemperatureRate float64     `json:"maxTemperatureRate"`
		MaxHumidityRate    float64     `json:"maxHumidityRate"`
	}
	normalized := make([]profileBasis, len(profiles))
	for i, profile := range profiles {
		normalized[i] = profileBasis{profile.CollectionCode, profile.SensitivityLevel, profile.TargetTemperatureRange, profile.TargetHumidityRange, profile.MaxTemperatureRate, profile.MaxHumidityRate}
	}
	payload, err := json.Marshal(struct {
		VenueTemperature Range               `json:"venueTemperature"`
		VenueHumidity    Range               `json:"venueHumidity"`
		Profiles         []profileBasis      `json:"profiles"`
		Basis            PlanBasis           `json:"basis"`
		Stages           []PlanStageSnapshot `json:"stages"`
	}{b.VenueTemperatureRange, b.VenueHumidityRange, normalized, b.PlanBasis, SnapshotPlan(b.Stages)})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (b *AcclimatizationBatch) RefreshPlanDigest() error {
	digest, err := b.CalculatePlanDigest()
	if err != nil {
		return err
	}
	b.PlanDigest = digest
	return nil
}
