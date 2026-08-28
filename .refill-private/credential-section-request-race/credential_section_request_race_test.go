package credentialsectionrequestrace_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

type controlledRepository struct {
	store.Repository
	credential   *domain.AdmissionCredential
	batch        *domain.AcclimatizationBatch
	mu           sync.Mutex
	calls        int
	firstEntered chan struct{}
	secondRead   chan struct{}
	releaseFirst chan struct{}
}

func (r *controlledRepository) GetCredentialBundle(context.Context, string) (*domain.AdmissionCredential, *domain.AcclimatizationBatch, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()

	if call == 1 {
		close(r.firstEntered)
		<-r.releaseFirst
	} else {
		close(r.secondRead)
	}
	return r.credential, r.batch, nil
}

type verificationResult struct {
	verification application.CredentialVerification
	err          error
}

func TestConcurrentCredentialVerificationKeepsEvidenceSectionIsolated(t *testing.T) {
	credential, batch := credentialBundle(t)
	repository := &controlledRepository{
		credential:   credential,
		batch:        batch,
		firstEntered: make(chan struct{}),
		secondRead:   make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	service := application.NewService(repository, nil, nil, nil, nil)

	profilesDone := make(chan verificationResult, 1)
	go func() {
		result, err := service.VerifyCredential(context.Background(), credential.ID, "profiles")
		profilesDone <- verificationResult{verification: result, err: err}
	}()
	<-repository.firstEntered

	reviewDone := make(chan verificationResult, 1)
	go func() {
		result, err := service.VerifyCredential(context.Background(), credential.ID, "review")
		reviewDone <- verificationResult{verification: result, err: err}
	}()
	<-repository.secondRead
	review := <-reviewDone
	close(repository.releaseFirst)
	profiles := <-profilesDone

	if review.err != nil {
		t.Fatalf("review verification failed: %v", review.err)
	}
	if profiles.err != nil {
		t.Fatalf("profiles verification failed: %v", profiles.err)
	}
	if _, ok := review.verification.EvidenceSection.(map[string]string); !ok {
		t.Fatalf("review request received %T, want review projection", review.verification.EvidenceSection)
	}
	if _, ok := profiles.verification.EvidenceSection.([]domain.EvidenceProfile); !ok {
		t.Fatalf("profiles request received %T, want profiles projection", profiles.verification.EvidenceSection)
	}
}

func credentialBundle(t *testing.T) (*domain.AdmissionCredential, *domain.AcclimatizationBatch) {
	t.Helper()
	completedAt := time.Date(2026, 8, 20, 10, 30, 0, 0, time.UTC)
	batch := &domain.AcclimatizationBatch{
		ID:        "batch_concurrent_verification",
		VenueID:   "venue_a",
		OwnerName: "owner_a",
		Status:    domain.BatchCertified,
		Version:   9,
		Profiles: []domain.ObjectMaterialProfile{{
			CollectionCode:         "collection_a",
			MaterialClasses:        []string{"paper"},
			SensitivityLevel:       domain.SensitivityHigh,
			TargetTemperatureRange: domain.Range{Min: 18, Max: 22},
			TargetHumidityRange:    domain.Range{Min: 40, Max: 55},
			MaxTemperatureRate:     1,
			MaxHumidityRate:        4,
		}},
		Stages: []domain.AcclimatizationStage{{
			ID:                     "stage_a",
			BatchID:                "batch_concurrent_verification",
			Sequence:               1,
			TargetTemperatureRange: domain.Range{Min: 18, Max: 22},
			TargetHumidityRange:    domain.Range{Min: 40, Max: 55},
			MinimumDuration:        30 * time.Minute,
			StabilityWindow:        10 * time.Minute,
			Status:                 domain.StageCompleted,
			Attempt:                1,
			CompletedAt:            &completedAt,
		}},
		Reviews: []domain.ReviewRecord{{
			ReviewerName: "reviewer_a",
			Decision:     domain.ReviewApproved,
			Reason:       "evidence accepted",
		}},
	}
	evidence, err := batch.EvidenceView()
	if err != nil {
		t.Fatalf("build evidence: %v", err)
	}
	digest, err := domain.DigestEvidence(evidence)
	if err != nil {
		t.Fatalf("digest evidence: %v", err)
	}
	credential := &domain.AdmissionCredential{
		ID:             "credential_a",
		BatchID:        batch.ID,
		BatchVersion:   batch.Version,
		EvidenceDigest: digest,
		Evidence:       evidence,
		ReviewerName:   "reviewer_a",
		IssuedAt:       completedAt.Add(time.Hour),
		SchemaVersion:  domain.CredentialSchemaVersion,
	}
	return credential, batch
}
