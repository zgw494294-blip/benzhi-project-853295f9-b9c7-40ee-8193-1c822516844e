package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

func (s *SQLiteStore) CreateBatch(_ context.Context, key, payloadHash string, batch *domain.AcclimatizationBatch) (Receipt, bool, error) {
	if key == "" {
		return Receipt{}, false, domain.Validation("Idempotency-Key", "创建批次必须提供幂等请求键")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, found, err := s.loadReceipt("create-batch", key, payloadHash); found || err != nil {
		return receipt, found, err
	}
	if _, exists := s.state.Batches[batch.ID]; exists {
		return Receipt{}, false, domain.Conflict("批次标识已存在")
	}
	s.state.Batches[batch.ID] = &batchRecord{Batch: cloneBatch(batch)}
	receipt := Receipt{Batch: cloneBatch(batch)}
	if err := s.saveReceipt("create-batch", key, payloadHash, receipt); err != nil {
		return Receipt{}, false, err
	}
	return receipt, false, s.commitLocked()
}
func (s *SQLiteStore) GetBatch(_ context.Context, id string) (*domain.AcclimatizationBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.state.Batches[id]
	if !ok {
		return nil, domain.NotFound("批次", id)
	}
	batch := cloneBatch(record.Batch)
	if err := batch.ValidateRecovered(); err != nil {
		return nil, err
	}
	return batch, nil
}
func (s *SQLiteStore) ListBatches(_ context.Context, filter BatchFilter) ([]*domain.AcclimatizationBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*domain.AcclimatizationBatch, 0)
	for _, record := range s.state.Batches {
		batch := cloneBatch(record.Batch)
		if err := batch.ValidateRecovered(); err != nil {
			return nil, err
		}
		if filter.Status != "" && batch.Status != filter.Status {
			continue
		}
		if filter.VenueID != "" && batch.VenueID != filter.VenueID {
			continue
		}
		if filter.OwnerName != "" && batch.OwnerName != filter.OwnerName {
			continue
		}
		if filter.PlannedStartFrom != nil && batch.PlannedStartAt.Before(*filter.PlannedStartFrom) {
			continue
		}
		if filter.PlannedStartTo != nil && batch.PlannedStartAt.After(*filter.PlannedStartTo) {
			continue
		}
		result = append(result, batch)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PlannedStartAt.Equal(result[j].PlannedStartAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].PlannedStartAt.Before(result[j].PlannedStartAt)
	})
	return result, nil
}
func (s *SQLiteStore) UpdateBatch(_ context.Context, id string, expected int64, operation, key, payloadHash string, mutate func(*domain.AcclimatizationBatch) error) (Receipt, bool, error) {
	return s.updateBatch(id, expected, operation, key, payloadHash, func(batch *domain.AcclimatizationBatch) (json.RawMessage, error) {
		return nil, mutate(batch)
	})
}
func (s *SQLiteStore) UpdateBatchWithResult(_ context.Context, id string, expected int64, operation, key, payloadHash string, mutate func(*domain.AcclimatizationBatch) (json.RawMessage, error)) (Receipt, bool, error) {
	return s.updateBatch(id, expected, operation, key, payloadHash, mutate)
}
func (s *SQLiteStore) updateBatch(id string, expected int64, operation, key, payloadHash string, mutate func(*domain.AcclimatizationBatch) (json.RawMessage, error)) (Receipt, bool, error) {
	if expected < 1 {
		return Receipt{}, false, domain.Validation("expectedVersion", "expectedVersion 必须大于 0")
	}
	if key == "" {
		return Receipt{}, false, domain.Validation("Idempotency-Key", "写操作必须提供幂等请求键")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scope := id + ":" + operation
	if receipt, found, err := s.loadReceipt(scope, key, payloadHash); found || err != nil {
		return receipt, found, err
	}
	record, ok := s.state.Batches[id]
	if !ok {
		return Receipt{}, false, domain.NotFound("批次", id)
	}
	batch := cloneBatch(record.Batch)
	if batch.Version != expected {
		return Receipt{}, false, domain.Conflict(fmt.Sprintf("版本冲突：当前版本为 %d", batch.Version))
	}
	result, err := mutate(batch)
	if err != nil {
		return Receipt{}, false, err
	}
	batch.Version++
	batch.UpdatedAt = time.Now().UTC()
	s.state.Batches[id] = &batchRecord{Batch: cloneBatch(batch)}
	receipt := Receipt{Batch: cloneBatch(batch), Result: append(json.RawMessage(nil), result...)}
	if err := s.saveReceipt(scope, key, payloadHash, receipt); err != nil {
		return Receipt{}, false, err
	}
	return receipt, false, s.commitLocked()
}
func (s *SQLiteStore) IssueCredential(_ context.Context, id string, expected int64, operation, key, payloadHash string, credential domain.AdmissionCredential) (Receipt, bool, error) {
	if expected < 1 {
		return Receipt{}, false, domain.Validation("expectedVersion", "expectedVersion 必须大于 0")
	}
	if key == "" {
		return Receipt{}, false, domain.Validation("Idempotency-Key", "签发凭据必须提供幂等请求键")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scope := id + ":" + operation
	if receipt, found, err := s.loadReceipt(scope, key, payloadHash); found || err != nil {
		return receipt, found, err
	}
	record, ok := s.state.Batches[id]
	if !ok {
		return Receipt{}, false, domain.NotFound("批次", id)
	}
	batch := cloneBatch(record.Batch)
	if batch.Version != expected {
		return Receipt{}, false, domain.Conflict(fmt.Sprintf("版本冲突：当前版本为 %d", batch.Version))
	}
	if err := batch.MarkCertified(credential.ID); err != nil {
		return Receipt{}, false, err
	}
	batch.Version++
	batch.UpdatedAt = credential.IssuedAt.UTC()
	credential.BatchVersion = batch.Version
	if _, exists := s.state.ByBatch[id]; exists {
		return Receipt{}, false, domain.Conflict("批次已经签发凭据")
	}
	s.state.Batches[id] = &batchRecord{Batch: cloneBatch(batch)}
	s.state.Credentials[credential.ID] = &domainCredential{Credential: cloneCredential(&credential)}
	s.state.ByBatch[id] = credential.ID
	receipt := Receipt{Batch: cloneBatch(batch), Credential: cloneCredential(&credential)}
	if err := s.saveReceipt(scope, key, payloadHash, receipt); err != nil {
		return Receipt{}, false, err
	}
	return receipt, false, s.commitLocked()
}
func (s *SQLiteStore) GetCredentialByBatch(_ context.Context, batchID string) (*domain.AdmissionCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.state.ByBatch[batchID]
	if !ok {
		return nil, domain.NotFound("准入凭据", batchID)
	}
	return s.credentialLocked(id)
}
func (s *SQLiteStore) GetCredential(_ context.Context, id string) (*domain.AdmissionCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credentialLocked(id)
}
func (s *SQLiteStore) GetCredentialBundle(_ context.Context, id string) (*domain.AdmissionCredential, *domain.AcclimatizationBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.state.Credentials[id]
	if !ok {
		return nil, nil, domain.NotFound("准入凭据", id)
	}
	credential := cloneCredential(record.Credential)
	if digest, err := domain.DigestEvidence(credential.Evidence); err != nil || digest != credential.EvidenceDigest {
		return credential, nil, domain.Integrity("凭据证据摘要校验失败")
	}
	batchRecord, ok := s.state.Batches[credential.BatchID]
	if !ok {
		return credential, nil, domain.Integrity("凭据关联批次不存在")
	}
	batch := cloneBatch(batchRecord.Batch)
	if err := batch.ValidateRecovered(); err != nil {
		return credential, batch, err
	}
	return credential, batch, nil
}
func (s *SQLiteStore) credentialLocked(id string) (*domain.AdmissionCredential, error) {
	record, ok := s.state.Credentials[id]
	if !ok {
		return nil, domain.NotFound("准入凭据", id)
	}
	credential := cloneCredential(record.Credential)
	digest, err := domain.DigestEvidence(credential.Evidence)
	if err != nil {
		return nil, err
	}
	if digest != credential.EvidenceDigest {
		return nil, domain.Integrity("凭据证据摘要校验失败")
	}
	return credential, nil
}
func (s *SQLiteStore) loadReceipt(scope, key, payloadHash string) (Receipt, bool, error) {
	record, ok := s.state.Idempotency[scope+"\x00"+key]
	if !ok {
		return Receipt{}, false, nil
	}
	if record.PayloadHash != payloadHash {
		return Receipt{}, false, domain.Conflict("同一 Idempotency-Key 不能用于不同请求载荷")
	}
	return cloneReceipt(record.Response), true, nil
}
func (s *SQLiteStore) saveReceipt(scope, key, payloadHash string, receipt Receipt) error {
	s.state.Idempotency[scope+"\x00"+key] = idempotencyRecord{PayloadHash: payloadHash, Response: cloneReceipt(receipt)}
	return nil
}
func cloneBatch(batch *domain.AcclimatizationBatch) *domain.AcclimatizationBatch {
	clone := *batch
	clone.Profiles = append([]domain.ObjectMaterialProfile(nil), batch.Profiles...)
	clone.Stages = append([]domain.AcclimatizationStage(nil), batch.Stages...)
	clone.Readings = append([]domain.Reading(nil), batch.Readings...)
	clone.Deviations = append([]domain.Deviation(nil), batch.Deviations...)
	clone.Reviews = append([]domain.ReviewRecord(nil), batch.Reviews...)
	clone.PlanBaseline = append([]domain.PlanStageSnapshot(nil), batch.PlanBaseline...)
	clone.CorrectionTasks = append([]domain.CorrectionTask(nil), batch.CorrectionTasks...)
	return &clone
}
func cloneCredential(credential *domain.AdmissionCredential) *domain.AdmissionCredential {
	data, _ := json.Marshal(credential)
	var clone domain.AdmissionCredential
	_ = json.Unmarshal(data, &clone)
	return &clone
}
func cloneReceipt(receipt Receipt) Receipt {
	result := Receipt{Result: append(json.RawMessage(nil), receipt.Result...)}
	if receipt.Batch != nil {
		result.Batch = cloneBatch(receipt.Batch)
	}
	if receipt.Credential != nil {
		result.Credential = cloneCredential(receipt.Credential)
	}
	return result
}
func mapSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE") {
		return domain.Conflict("唯一性约束冲突")
	}
	return err
}
