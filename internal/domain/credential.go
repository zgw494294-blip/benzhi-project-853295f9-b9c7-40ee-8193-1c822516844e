package domain

import "time"

const CredentialSchemaVersion = "acclimatization-evidence/v1"

type AdmissionCredential struct {
	ID             string    `json:"id"`
	BatchID        string    `json:"batchId"`
	BatchVersion   int64     `json:"batchVersion"`
	EvidenceDigest string    `json:"evidenceDigest"`
	Evidence       Evidence  `json:"evidence"`
	ReviewerName   string    `json:"reviewerName"`
	IssuedAt       time.Time `json:"issuedAt"`
	SchemaVersion  string    `json:"schemaVersion"`
}
