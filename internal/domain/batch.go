package domain

import (
	"fmt"
	"sort"
	"time"
)

type BatchStatus string

const (
	BatchDraft      BatchStatus = "draft"
	BatchReady      BatchStatus = "ready"
	BatchRunning    BatchStatus = "running"
	BatchIsolated   BatchStatus = "isolated"
	BatchReview     BatchStatus = "review"
	BatchCorrection BatchStatus = "correction"
	BatchApproved   BatchStatus = "approved"
	BatchCertified  BatchStatus = "certified"
)

type AcclimatizationBatch struct {
	ID                    string                  `json:"id"`
	VenueID               string                  `json:"venueId"`
	PlannedStartAt        time.Time               `json:"plannedStartAt"`
	OwnerName             string                  `json:"ownerName"`
	VenueTemperatureRange Range                   `json:"venueTemperatureRange"`
	VenueHumidityRange    Range                   `json:"venueHumidityRange"`
	Status                BatchStatus             `json:"status"`
	CurrentStageID        string                  `json:"currentStageId,omitempty"`
	Version               int64                   `json:"version"`
	CreatedAt             time.Time               `json:"createdAt"`
	UpdatedAt             time.Time               `json:"updatedAt"`
	Profiles              []ObjectMaterialProfile `json:"profiles"`
	Stages                []AcclimatizationStage  `json:"stages"`
	Readings              []Reading               `json:"readings"`
	Deviations            []Deviation             `json:"deviations"`
	Reviews               []ReviewRecord          `json:"reviews"`
	PlanDigest            string                  `json:"planDigest,omitempty"`
	PlanBasis             PlanBasis               `json:"planBasis,omitempty"`
	PlanBaseline          []PlanStageSnapshot     `json:"planBaseline,omitempty"`
	CorrectionTasks       []CorrectionTask        `json:"correctionTasks"`
	CorrectionRequired    string                  `json:"correctionRequired,omitempty"`
	CorrectionStageID     string                  `json:"correctionStageId,omitempty"`
	CredentialID          string                  `json:"credentialId,omitempty"`
}

func NewBatch(id, venueID, owner string, planned time.Time, temp, humidity Range, now time.Time) (*AcclimatizationBatch, error) {
	if err := ValidateName("venueId", venueID, 80); err != nil {
		return nil, err
	}
	if err := ValidateName("ownerName", owner, 80); err != nil {
		return nil, err
	}
	if err := ValidateTimestamp("plannedStartAt", planned); err != nil {
		return nil, err
	}
	if err := temp.Validate("venueTemperatureRange", -10, 40); err != nil {
		return nil, err
	}
	if err := humidity.Validate("venueHumidityRange", 10, 80); err != nil {
		return nil, err
	}
	return &AcclimatizationBatch{ID: id, VenueID: venueID, OwnerName: owner, PlannedStartAt: planned.UTC(), VenueTemperatureRange: temp, VenueHumidityRange: humidity, Status: BatchDraft, Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}

func (b *AcclimatizationBatch) AddProfile(profile ObjectMaterialProfile) error {
	if b.Status != BatchDraft {
		return InvalidState("只有 draft 批次可以登记材料档案")
	}
	for _, existing := range b.Profiles {
		if existing.CollectionCode == profile.CollectionCode {
			return Conflict("同一批次内 collectionCode 不能重复")
		}
	}
	b.Profiles = append(b.Profiles, profile)
	sort.Slice(b.Profiles, func(i, j int) bool { return b.Profiles[i].CollectionCode < b.Profiles[j].CollectionCode })
	return nil
}

func (b *AcclimatizationBatch) AddProfiles(profiles []ObjectMaterialProfile) error {
	if b.Status != BatchDraft {
		return InvalidState("只有 draft 批次可以登记材料档案")
	}
	if len(profiles) == 0 {
		return Validation("profiles", "至少提供一条材料档案")
	}
	seen := make(map[string]struct{}, len(b.Profiles)+len(profiles))
	all := append([]ObjectMaterialProfile(nil), b.Profiles...)
	for _, existing := range b.Profiles {
		seen[existing.CollectionCode] = struct{}{}
	}
	for i, profile := range profiles {
		if _, exists := seen[profile.CollectionCode]; exists {
			return Validation(fmt.Sprintf("profiles[%d].collectionCode", i), "同一批次内 collectionCode 不能重复")
		}
		seen[profile.CollectionCode] = struct{}{}
		all = append(all, profile)
	}
	if err := ValidateProfileIntersection(all); err != nil {
		return err
	}
	b.Profiles = all
	sort.Slice(b.Profiles, func(i, j int) bool { return b.Profiles[i].CollectionCode < b.Profiles[j].CollectionCode })
	return nil
}

func (b *AcclimatizationBatch) SetPlan(stages []AcclimatizationStage) error {
	return b.SetPlanWithBasis(stages, PlanBasis{})
}

func (b *AcclimatizationBatch) SetPlanWithBasis(stages []AcclimatizationStage, basis PlanBasis) error {
	if b.Status != BatchDraft {
		return InvalidState("只有 draft 批次可以生成方案")
	}
	if len(b.Profiles) == 0 {
		return InvalidState("生成方案前必须登记材料档案")
	}
	if len(stages) < 2 {
		return Validation("stages", "驯化方案至少包含两个阶段")
	}
	for i := range stages {
		if stages[i].Sequence != i+1 {
			return Validation("sequence", "阶段必须按连续序号排列")
		}
		if err := stages[i].Validate(); err != nil {
			return err
		}
	}
	b.Stages = stages
	b.PlanBasis = basis
	b.PlanBaseline = SnapshotPlan(stages)
	return b.RefreshPlanDigest()
}

func (b *AcclimatizationBatch) ReviseStage(stageID string, duration, window time.Duration) error {
	if b.Status != BatchDraft && b.Status != BatchIsolated {
		return InvalidState("当前状态不允许修订阶段")
	}
	stage := b.StageByID(stageID)
	if stage == nil {
		return NotFound("阶段", stageID)
	}
	copy := *stage
	copy.MinimumDuration, copy.StabilityWindow = duration, window
	if err := copy.Validate(); err != nil {
		return err
	}
	stage.MinimumDuration, stage.StabilityWindow = duration, window
	return b.RefreshPlanDigest()
}

func (b *AcclimatizationBatch) Freeze() error {
	return b.FreezeWithDigest(b.PlanDigest)
}

func (b *AcclimatizationBatch) FreezeWithDigest(digest string) error {
	if b.Status != BatchDraft {
		return InvalidState("只有 draft 批次可以冻结方案")
	}
	if len(b.Stages) == 0 {
		return InvalidState("冻结前必须生成阶段方案")
	}
	current, err := b.CalculatePlanDigest()
	if err != nil {
		return err
	}
	if digest == "" || digest != b.PlanDigest || digest != current {
		return Conflict("planDigest 与当前方案不一致，请重新查询差异后确认")
	}
	b.Status = BatchReady
	b.CurrentStageID = b.Stages[0].ID
	return nil
}

func (b *AcclimatizationBatch) Start(now time.Time) error {
	if b.Status != BatchReady && b.Status != BatchCorrection {
		return InvalidState("只有 ready 或 correction 批次可以启动")
	}
	stage := b.StageByID(b.CurrentStageID)
	if stage == nil {
		return Integrity("当前阶段不存在")
	}
	stage.Status = StageActive
	started := now.UTC()
	stage.StartedAt = &started
	b.Status = BatchRunning
	return nil
}

func (b *AcclimatizationBatch) Isolate() error {
	if b.Status != BatchRunning {
		return InvalidState("只有 running 批次可以隔离")
	}
	b.Status = BatchIsolated
	return nil
}

func (b *AcclimatizationBatch) CompleteCurrentStage(at time.Time) error {
	if b.Status != BatchRunning {
		return InvalidState("只有 running 批次可以完成阶段")
	}
	stage := b.StageByID(b.CurrentStageID)
	if stage == nil {
		return Integrity("当前阶段不存在")
	}
	stage.Status = StageCompleted
	completed := at.UTC()
	stage.CompletedAt = &completed
	if stage.Sequence < len(b.Stages) {
		next := &b.Stages[stage.Sequence]
		next.Status = StageActive
		next.StartedAt = &completed
		b.CurrentStageID = next.ID
	}
	return nil
}

func (b *AcclimatizationBatch) AllStagesCompleted() bool {
	if len(b.Stages) == 0 {
		return false
	}
	for i := range b.Stages {
		if b.Stages[i].Status != StageCompleted {
			return false
		}
	}
	return true
}

func (b *AcclimatizationBatch) StageByID(id string) *AcclimatizationStage {
	for i := range b.Stages {
		if b.Stages[i].ID == id {
			return &b.Stages[i]
		}
	}
	return nil
}

func (b *AcclimatizationBatch) ResolveDeviation(id string, evidence DeviationResolutionEvidence, checkpoint int, now time.Time) error {
	if b.Status != BatchIsolated {
		return InvalidState("只有 isolated 批次可以完成偏差处置")
	}
	if err := evidence.Validate(); err != nil {
		return err
	}
	open := 0
	for i := range b.Deviations {
		if b.Deviations[i].IsOpen() {
			open++
		}
	}
	if open != 1 {
		return InvalidState("必须且只能存在一个开放偏差才能执行处置")
	}
	var target *Deviation
	for i := range b.Deviations {
		if b.Deviations[i].ID == id {
			target = &b.Deviations[i]
			break
		}
	}
	if target == nil {
		return NotFound("偏差", id)
	}
	if !target.IsOpen() {
		return Conflict("偏差已经处置")
	}
	if checkpoint < 1 || checkpoint > len(b.Stages) || checkpoint > target.CheckpointSequence {
		return Validation("checkpointSequence", "检查点必须位于允许的已完成阶段之后")
	}
	resolved := now.UTC()
	target.ResolvedAt, target.Resolution = &resolved, evidence.Conclusion
	target.ResponsiblePerson = evidence.ResponsiblePerson
	target.EnvironmentRemediation = evidence.EnvironmentRemediation
	target.Conclusion = evidence.Conclusion
	target.CheckpointStageID = b.Stages[checkpoint-1].ID
	for i := checkpoint - 1; i < len(b.Stages); i++ {
		b.Stages[i].Status = StagePlanned
		b.Stages[i].StartedAt = nil
		b.Stages[i].CompletedAt = nil
		if i == checkpoint-1 {
			b.Stages[i].Attempt++
		}
	}
	b.CurrentStageID = b.Stages[checkpoint-1].ID
	b.Status = BatchReady
	return nil
}

func (b *AcclimatizationBatch) SubmitReview() error {
	if b.Status != BatchRunning && b.Status != BatchReady {
		return InvalidState("当前状态不能提交复核")
	}
	if !b.AllStagesCompleted() {
		return InvalidState("全部阶段达标后才能提交复核")
	}
	for _, d := range b.Deviations {
		if d.IsOpen() {
			return InvalidState("存在开放偏差，不能提交复核")
		}
	}
	for _, task := range b.CorrectionTasks {
		if task.CompletedAt == nil {
			return InvalidState("补正任务尚未全部完成")
		}
	}
	b.Status = BatchReview
	return nil
}

func (b *AcclimatizationBatch) DecideReview(record ReviewRecord) error {
	if b.Status != BatchReview {
		return InvalidState("只有 review 批次可以作出复核决定")
	}
	if record.Decision != ReviewApproved && record.Decision != ReviewReturned {
		return Validation("decision", "决定必须是 approved 或 returned")
	}
	if err := ValidateName("reviewerName", record.ReviewerName, 80); err != nil {
		return err
	}
	if err := ValidateName("reason", record.Reason, 500); err != nil {
		return err
	}
	if record.Decision == ReviewReturned {
		stage := b.StageByID(record.RequiredStageID)
		if stage == nil {
			return Validation("requiredStageId", "退回时必须指定有效阶段")
		}
		if err := ValidateName("requiredAction", record.RequiredAction, 500); err != nil {
			return err
		}
		stage.Status = StagePlanned
		stage.StartedAt = nil
		stage.CompletedAt = nil
		stage.Attempt++
		b.CurrentStageID = stage.ID
		b.Status, b.CorrectionStageID, b.CorrectionRequired = BatchCorrection, record.RequiredStageID, record.RequiredAction
		b.CorrectionTasks = append(b.CorrectionTasks, CorrectionTask{ID: record.ID, RequiredStageID: record.RequiredStageID, RequiredAction: record.RequiredAction, SubmissionVersion: record.SubmissionVersion, RequiredAttempt: stage.Attempt, CreatedAt: record.DecidedAt})
	} else {
		b.Status = BatchApproved
	}
	b.Reviews = append(b.Reviews, record)
	return nil
}

func (b *AcclimatizationBatch) CompleteCorrection(now time.Time) error {
	if b.Status != BatchRunning && b.Status != BatchReady {
		return InvalidState("补正重跑完成后才能确认补正任务")
	}
	for i := range b.CorrectionTasks {
		if b.CorrectionTasks[i].CompletedAt != nil {
			continue
		}
		if err := b.CorrectionTasks[i].ValidateCompletion(b); err != nil {
			return err
		}
		completed := now.UTC()
		b.CorrectionTasks[i].CompletedAt = &completed
	}
	b.Status = BatchReady
	b.CorrectionRequired = ""
	b.CorrectionStageID = ""
	_ = now
	return nil
}

func (b *AcclimatizationBatch) MarkCertified(credentialID string) error {
	if b.Status != BatchApproved {
		return InvalidState("只有 approved 批次可以签发凭据")
	}
	if b.CredentialID != "" {
		return Conflict("批次已经签发凭据")
	}
	b.Status, b.CredentialID = BatchCertified, credentialID
	return nil
}

func (b *AcclimatizationBatch) ValidateRecovered() error {
	if b.ID == "" || b.Version < 1 {
		return Integrity("批次标识或版本无效")
	}
	for i := range b.Stages {
		if b.Stages[i].Sequence != i+1 {
			return Integrity(fmt.Sprintf("阶段 %d 顺序损坏", i+1))
		}
	}
	return nil
}
