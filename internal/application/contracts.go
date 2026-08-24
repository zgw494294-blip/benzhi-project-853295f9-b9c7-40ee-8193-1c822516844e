package application

import (
	"context"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/policy"
	"collection-acclimatization-pass/internal/store"
)

type BatchRepository = store.Repository

type StagePlanner interface {
	Generate(*domain.AcclimatizationBatch) ([]domain.AcclimatizationStage, policy.PlanBasis, error)
}

type ReadingEvaluator interface {
	Evaluate(*domain.AcclimatizationBatch, domain.AcclimatizationStage, domain.Reading) (policy.Evaluation, error)
}

type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Mutation struct {
	ExpectedVersion int64
	IdempotencyKey  string
}

type BatchResult struct {
	Batch          *domain.AcclimatizationBatch `json:"batch"`
	Replayed       bool                         `json:"replayed"`
	Decision       *policy.Evaluation           `json:"decision,omitempty"`
	Plan           *PlanView                    `json:"plan,omitempty"`
	ReadingResults []ReadingItemResult          `json:"readingResults,omitempty"`
	CurrentStageID string                       `json:"currentStageId,omitempty"`
	SkippedCount   int                          `json:"skippedCount,omitempty"`
}

type CredentialResult struct {
	Batch      *domain.AcclimatizationBatch `json:"batch"`
	Credential *domain.AdmissionCredential  `json:"credential"`
	Replayed   bool                         `json:"replayed"`
}

type ServiceAPI interface {
	CreateBatch(context.Context, CreateBatchInput, string) (BatchResult, error)
	GetBatch(context.Context, string) (*domain.AcclimatizationBatch, error)
	ListBatches(context.Context, BatchListQuery) (BatchListResult, error)
	AddProfile(context.Context, string, AddProfileInput, Mutation) (BatchResult, error)
	AddProfiles(context.Context, string, []AddProfileInput, Mutation) (BatchResult, error)
	GeneratePlan(context.Context, string, Mutation) (BatchResult, error)
	GetPlan(context.Context, string) (PlanView, error)
	ReviseStage(context.Context, string, string, ReviseStageInput, Mutation) (BatchResult, error)
	FreezePlan(context.Context, string, string, Mutation) (BatchResult, error)
	StartBatch(context.Context, string, Mutation) (BatchResult, error)
	SubmitReading(context.Context, string, ReadingInput, Mutation) (BatchResult, error)
	SubmitReadings(context.Context, string, []ReadingInput, Mutation) (BatchResult, error)
	QueryReadings(context.Context, string, ReadingQuery) (ReadingQueryResult, error)
	ListDeviations(context.Context, string) ([]domain.Deviation, error)
	ResolveDeviation(context.Context, string, string, ResolveDeviationInput, Mutation) (BatchResult, error)
	SubmitReview(context.Context, string, Mutation) (BatchResult, error)
	DecideReview(context.Context, string, ReviewDecisionInput, Mutation) (BatchResult, error)
	CompleteCorrection(context.Context, string, Mutation) (BatchResult, error)
	GetCorrectionProgress(context.Context, string) (CorrectionProgress, error)
	IssueCredential(context.Context, string, string, Mutation) (CredentialResult, error)
	GetCredentialByBatch(context.Context, string) (*domain.AdmissionCredential, error)
	GetCredential(context.Context, string) (*domain.AdmissionCredential, error)
	VerifyCredential(context.Context, string, string) (CredentialVerification, error)
	VerifyCredentialByBatch(context.Context, string, string) (CredentialVerification, error)
}
