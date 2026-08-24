package application

import (
	"context"
	"strings"
	"time"

	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/store"
)

type BatchListQuery struct {
	Status           domain.BatchStatus
	VenueID          string
	OwnerName        string
	PlannedStartFrom *time.Time
	PlannedStartTo   *time.Time
	Cursor           string
	Limit            int
}

type BatchTodo struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	StageID string `json:"stageId,omitempty"`
}

type BatchListItem struct {
	*domain.AcclimatizationBatch
	CurrentStageSequence int         `json:"currentStageSequence"`
	StageTotal           int         `json:"stageTotal"`
	OpenDeviationCount   int         `json:"openDeviationCount"`
	Todos                []BatchTodo `json:"todos"`
}

type BatchListResult struct {
	Items         []BatchListItem            `json:"items"`
	NextCursor    string                     `json:"nextCursor,omitempty"`
	Total         int                        `json:"total"`
	StatusCounts  map[domain.BatchStatus]int `json:"statusCounts"`
	UpcomingCount int                        `json:"upcomingCount"`
}

func (s *Service) ListBatches(ctx context.Context, query BatchListQuery) (BatchListResult, error) {
	if query.Status != "" && !validBatchStatus(query.Status) {
		return BatchListResult{}, domain.Validation("status", "未知批次状态")
	}
	if query.PlannedStartFrom != nil && query.PlannedStartTo != nil && query.PlannedStartTo.Before(*query.PlannedStartFrom) {
		return BatchListResult{}, domain.Validation("plannedStartAtTo", "结束时间不能早于开始时间")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 {
		return BatchListResult{}, domain.Validation("limit", "分页条数必须在 1 到 100 之间")
	}
	batches, err := s.store.ListBatches(ctx, store.BatchFilter{Status: query.Status, VenueID: strings.TrimSpace(query.VenueID), OwnerName: strings.TrimSpace(query.OwnerName), PlannedStartFrom: query.PlannedStartFrom, PlannedStartTo: query.PlannedStartTo})
	if err != nil {
		return BatchListResult{}, err
	}
	result := BatchListResult{Items: []BatchListItem{}, Total: len(batches), StatusCounts: map[domain.BatchStatus]int{}}
	now, upcomingEnd := s.clock.Now(), s.clock.Now().Add(7*24*time.Hour)
	for _, batch := range batches {
		result.StatusCounts[batch.Status]++
		if !batch.PlannedStartAt.Before(now) && !batch.PlannedStartAt.After(upcomingEnd) && (batch.Status == domain.BatchDraft || batch.Status == domain.BatchReady) {
			result.UpcomingCount++
		}
	}
	start := 0
	if query.Cursor != "" {
		found := false
		for i, batch := range batches {
			if batch.ID == query.Cursor {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return BatchListResult{}, domain.Validation("cursor", "游标不属于当前筛选结果")
		}
	}
	end := start + query.Limit
	if end > len(batches) {
		end = len(batches)
	}
	for _, batch := range batches[start:end] {
		result.Items = append(result.Items, summarizeBatch(batch))
	}
	if end < len(batches) && end > start {
		result.NextCursor = batches[end-1].ID
	}
	return result, nil
}

func validBatchStatus(status domain.BatchStatus) bool {
	switch status {
	case domain.BatchDraft, domain.BatchReady, domain.BatchRunning, domain.BatchIsolated, domain.BatchReview, domain.BatchCorrection, domain.BatchApproved, domain.BatchCertified:
		return true
	}
	return false
}

func summarizeBatch(batch *domain.AcclimatizationBatch) BatchListItem {
	item := BatchListItem{AcclimatizationBatch: batch, StageTotal: len(batch.Stages), Todos: []BatchTodo{}}
	if stage := batch.StageByID(batch.CurrentStageID); stage != nil {
		item.CurrentStageSequence = stage.Sequence
	}
	for _, deviation := range batch.Deviations {
		if deviation.IsOpen() {
			item.OpenDeviationCount++
		}
	}
	switch batch.Status {
	case domain.BatchDraft:
		if len(batch.Profiles) == 0 {
			item.Todos = append(item.Todos, BatchTodo{"register_profiles", "登记材料档案", ""})
		} else if len(batch.Stages) == 0 {
			item.Todos = append(item.Todos, BatchTodo{"generate_plan", "生成驯化方案", ""})
		} else {
			item.Todos = append(item.Todos, BatchTodo{"confirm_plan", "确认并冻结方案", batch.CurrentStageID})
		}
	case domain.BatchReady:
		item.Todos = append(item.Todos, BatchTodo{"start", "启动当前阶段", batch.CurrentStageID})
	case domain.BatchRunning:
		item.Todos = append(item.Todos, BatchTodo{"collect_readings", "采集环境读数", batch.CurrentStageID})
	case domain.BatchIsolated:
		item.Todos = append(item.Todos, BatchTodo{"resolve_deviation", "处置开放偏差", batch.CurrentStageID})
	case domain.BatchReview:
		item.Todos = append(item.Todos, BatchTodo{"review_decision", "完成保护复核", ""})
	case domain.BatchCorrection:
		item.Todos = append(item.Todos, BatchTodo{"run_correction", "执行补正重跑", batch.CorrectionStageID})
	case domain.BatchApproved:
		item.Todos = append(item.Todos, BatchTodo{"issue_credential", "签发准入凭据", ""})
	}
	return item
}
