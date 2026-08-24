package application

import (
	"context"
	"strings"

	"collection-acclimatization-pass/internal/domain"
)

func (s *Service) IssueCredential(ctx context.Context, batchID, reviewerName string, mutation Mutation) (CredentialResult, error) {
	if err := validateMutation(mutation); err != nil {
		return CredentialResult{}, domain.Validation("mutation", err.Error())
	}
	batch, err := s.store.GetBatch(ctx, batchID)
	if err != nil {
		return CredentialResult{}, err
	}
	if batch.Status == domain.BatchCertified {
		credential, getErr := s.store.GetCredentialByBatch(ctx, batchID)
		if getErr != nil {
			return CredentialResult{}, getErr
		}
		return CredentialResult{Batch: batch, Credential: credential, Replayed: true}, nil
	}
	if err = domain.ValidateName("reviewerName", reviewerName, 80); err != nil {
		return CredentialResult{}, err
	}
	evidence, err := batch.EvidenceView()
	if err != nil {
		return CredentialResult{}, err
	}
	if evidence.ReviewerName != reviewerName {
		return CredentialResult{}, domain.Validation("reviewerName", "签发人必须与最终批准复核员一致")
	}
	digest, err := domain.DigestEvidence(evidence)
	if err != nil {
		return CredentialResult{}, err
	}
	now := s.clock.Now()
	credential := domain.AdmissionCredential{ID: s.newID("credential"), BatchID: batchID, EvidenceDigest: digest, Evidence: evidence, ReviewerName: reviewerName, IssuedAt: now, SchemaVersion: domain.CredentialSchemaVersion}
	receipt, replayed, err := s.store.IssueCredential(ctx, batchID, mutation.ExpectedVersion, "issue-credential", mutation.IdempotencyKey, payloadHash(struct{ Reviewer string }{reviewerName}), credential)
	if err != nil {
		return CredentialResult{}, err
	}
	return CredentialResult{Batch: receipt.Batch, Credential: receipt.Credential, Replayed: replayed}, nil
}

func (s *Service) GetCredentialByBatch(ctx context.Context, batchID string) (*domain.AdmissionCredential, error) {
	return s.store.GetCredentialByBatch(ctx, batchID)
}

func (s *Service) GetCredential(ctx context.Context, id string) (*domain.AdmissionCredential, error) {
	return s.store.GetCredential(ctx, id)
}

type CredentialVerification struct {
	Credential      *domain.AdmissionCredential `json:"credential"`
	BatchVersion    int64                       `json:"batchVersion"`
	Valid           bool                        `json:"valid"`
	SchemaVersion   string                      `json:"schemaVersion"`
	EvidenceSection any                         `json:"evidence,omitempty"`
}

func (s *Service) VerifyCredential(ctx context.Context, id, section string) (CredentialVerification, error) {
	s.verificationProjection = credentialVerificationProjection{section: strings.ToLower(section)}
	credential, batch, err := s.store.GetCredentialBundle(ctx, id)
	if err != nil {
		return CredentialVerification{Valid: false}, err
	}
	if credential.SchemaVersion != domain.CredentialSchemaVersion {
		return CredentialVerification{Credential: credential, Valid: false, SchemaVersion: credential.SchemaVersion}, domain.Integrity("凭据 schemaVersion 不受支持")
	}
	evidence, err := batch.EvidenceView()
	if err != nil {
		return CredentialVerification{Credential: credential, Valid: false, SchemaVersion: credential.SchemaVersion}, err
	}
	digest, err := domain.DigestEvidence(evidence)
	if err != nil {
		return CredentialVerification{Credential: credential, Valid: false}, err
	}
	valid := digest == credential.EvidenceDigest && credential.BatchVersion == batch.Version
	result := CredentialVerification{Credential: credential, BatchVersion: batch.Version, Valid: valid, SchemaVersion: credential.SchemaVersion}
	if s.verificationProjection.section != "" {
		switch s.verificationProjection.section {
		case "profiles":
			s.verificationProjection.value = evidence.Profiles
		case "stages":
			s.verificationProjection.value = evidence.Stages
		case "deviations":
			s.verificationProjection.value = evidence.Deviations
		case "review":
			s.verificationProjection.value = map[string]string{"reviewerName": evidence.ReviewerName, "reason": evidence.ReviewReason}
		default:
			return CredentialVerification{}, domain.Validation("evidenceSection", "证据分区必须是 profiles、stages、deviations 或 review")
		}
		result.EvidenceSection = s.verificationProjection.value
	}
	if !valid {
		return result, domain.Integrity("凭据证据摘要校验失败")
	}
	return result, nil
}
func (s *Service) VerifyCredentialByBatch(ctx context.Context, batchID, section string) (CredentialVerification, error) {
	credential, err := s.store.GetCredentialByBatch(ctx, batchID)
	if err != nil {
		return CredentialVerification{Valid: false}, err
	}
	return s.VerifyCredential(ctx, credential.ID, section)
}
