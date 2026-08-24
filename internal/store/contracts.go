package store

import (
	"context"
	"encoding/json"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

type Receipt struct {
	Batch      *domain.AcclimatizationBatch `json:"batch"`
	Credential *domain.AdmissionCredential  `json:"credential,omitempty"`
	Result     json.RawMessage              `json:"result,omitempty"`
}

type BatchFilter struct {
	Status           domain.BatchStatus
	VenueID          string
	OwnerName        string
	PlannedStartFrom *time.Time
	PlannedStartTo   *time.Time
}

type Repository interface {
	CreateBatch(context.Context, string, string, *domain.AcclimatizationBatch) (Receipt, bool, error)
	GetBatch(context.Context, string) (*domain.AcclimatizationBatch, error)
	ListBatches(context.Context, BatchFilter) ([]*domain.AcclimatizationBatch, error)
	UpdateBatch(context.Context, string, int64, string, string, string, func(*domain.AcclimatizationBatch) error) (Receipt, bool, error)
	UpdateBatchWithResult(context.Context, string, int64, string, string, string, func(*domain.AcclimatizationBatch) (json.RawMessage, error)) (Receipt, bool, error)
	IssueCredential(context.Context, string, int64, string, string, string, domain.AdmissionCredential) (Receipt, bool, error)
	GetCredentialByBatch(context.Context, string) (*domain.AdmissionCredential, error)
	GetCredential(context.Context, string) (*domain.AdmissionCredential, error)
	GetCredentialBundle(context.Context, string) (*domain.AdmissionCredential, *domain.AcclimatizationBatch, error)
	Close() error
}
