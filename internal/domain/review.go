package domain

import "time"

type ReviewDecision string

const (
	ReviewReturned ReviewDecision = "returned"
	ReviewApproved ReviewDecision = "approved"
)

type ReviewRecord struct {
	ID                string         `json:"id"`
	BatchID           string         `json:"batchId"`
	ReviewerName      string         `json:"reviewerName"`
	Decision          ReviewDecision `json:"decision"`
	Reason            string         `json:"reason"`
	RequiredStageID   string         `json:"requiredStageId,omitempty"`
	RequiredAction    string         `json:"requiredAction,omitempty"`
	SubmittedAt       time.Time      `json:"submittedAt"`
	DecidedAt         time.Time      `json:"decidedAt"`
	SubmissionVersion int64          `json:"submissionVersion"`
}
