package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
)

type Server struct {
	app  application.ServiceAPI
	http *http.Server
}

func New(app application.ServiceAPI) *Server {
	s := &Server{app: app}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/acclimatization-batches", s.batches)
	mux.HandleFunc("/v1/acclimatization-batches/", s.batchResource)
	mux.HandleFunc("/v1/admission-credentials/", s.credentialResource)
	s.http = &http.Server{Handler: logging(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	return s
}

func (s *Server) Handler() http.Handler             { return s.http.Handler }
func (s *Server) Serve(listener net.Listener) error { return s.http.Serve(listener) }
func (s *Server) ListenAndServe(addr string) error {
	s.http.Addr = addr
	return s.http.ListenAndServe()
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

func (s *Server) batches(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/acclimatization-batches" {
		writeError(w, domain.NotFound("路径", r.URL.Path))
		return
	}
	if r.Method == http.MethodGet {
		query, err := parseBatchQuery(r)
		if err != nil {
			writeError(w, err)
			return
		}
		result, err := s.app.ListBatches(r.Context(), query)
		writeResult(w, http.StatusOK, result, err)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost, http.MethodGet)
		return
	}
	var input application.CreateBatchInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.CreateBatch(r.Context(), input, r.Header.Get("Idempotency-Key"))
	writeResult(w, http.StatusCreated, result, err)
}

func (s *Server) batchResource(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/acclimatization-batches/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, domain.NotFound("批次", ""))
		return
	}
	batchID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		batch, err := s.app.GetBatch(r.Context(), batchID)
		writeResult(w, http.StatusOK, batch, err)
		return
	}
	if len(parts) == 2 && parts[1] == "readings" && r.Method == http.MethodGet {
		query, err := parseReadingQuery(r)
		if err != nil {
			writeError(w, err)
			return
		}
		result, err := s.app.QueryReadings(r.Context(), batchID, query)
		writeResult(w, http.StatusOK, result, err)
		return
	}
	if len(parts) == 1 {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	action := parts[1]
	if action == "deviations" && len(parts) == 2 && r.Method == http.MethodGet {
		list, err := s.app.ListDeviations(r.Context(), batchID)
		writeResult(w, http.StatusOK, list, err)
		return
	}
	if action == "credential" && r.Method == http.MethodGet {
		if r.URL.Query().Get("verify") == "true" {
			section := r.URL.Query().Get("evidenceSection")
			if section == "" {
				section = r.URL.Query().Get("section")
			}
			result, err := s.app.VerifyCredentialByBatch(r.Context(), batchID, section)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		credential, err := s.app.GetCredentialByBatch(r.Context(), batchID)
		writeResult(w, http.StatusOK, credential, err)
		return
	}
	if action == "plan" && len(parts) == 3 && parts[2] == "diff" && r.Method == http.MethodGet {
		result, err := s.app.GetPlan(r.Context(), batchID)
		writeResult(w, http.StatusOK, result, err)
		return
	}
	if action == "correction" && len(parts) == 2 && r.Method == http.MethodGet {
		result, err := s.app.GetCorrectionProgress(r.Context(), batchID)
		writeResult(w, http.StatusOK, result, err)
		return
	}
	mutation, err := parseMutation(r)
	if err != nil {
		writeError(w, err)
		return
	}
	switch action {
	case "profiles":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var raw json.RawMessage
		if err := decodeJSON(r, &raw); err != nil {
			writeError(w, err)
			return
		}
		var inputs []application.AddProfileInput
		if len(raw) > 0 && raw[0] == '[' {
			if err := json.Unmarshal(raw, &inputs); err != nil {
				writeError(w, domain.Validation("profiles", "批量档案格式无效"))
				return
			}
		} else {
			var wrapper struct {
				Profiles []application.AddProfileInput `json:"profiles"`
			}
			if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Profiles != nil {
				inputs = wrapper.Profiles
			} else {
				var input application.AddProfileInput
				if err := json.Unmarshal(raw, &input); err != nil {
					writeError(w, domain.Validation("body", "材料档案格式无效"))
					return
				}
				inputs = []application.AddProfileInput{input}
			}
		}
		result, err := s.app.AddProfiles(r.Context(), batchID, inputs, mutation)
		writeResult(w, http.StatusOK, result, err)
	case "plan":
		if len(parts) == 2 && r.Method == http.MethodPost {
			result, err := s.app.GeneratePlan(r.Context(), batchID, mutation)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		if len(parts) == 3 && parts[2] == "freeze" && r.Method == http.MethodPost {
			var input struct {
				PlanDigest string `json:"planDigest"`
			}
			if err := decodeJSON(r, &input); err != nil {
				writeError(w, err)
				return
			}
			result, err := s.app.FreezePlan(r.Context(), batchID, input.PlanDigest, mutation)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		methodNotAllowed(w, http.MethodPost)
	case "stages":
		if len(parts) != 3 || r.Method != http.MethodPatch {
			methodNotAllowed(w, http.MethodPatch)
			return
		}
		var input application.ReviseStageInput
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.app.ReviseStage(r.Context(), batchID, parts[2], input, mutation)
		writeResult(w, http.StatusOK, result, err)
	case "start":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.StartBatch(r.Context(), batchID, mutation)
		writeResult(w, http.StatusOK, result, err)
	case "readings":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var raw json.RawMessage
		if err := decodeJSON(r, &raw); err != nil {
			writeError(w, err)
			return
		}
		var inputs []application.ReadingInput
		if len(raw) > 0 && raw[0] == '[' {
			if err := json.Unmarshal(raw, &inputs); err != nil {
				writeError(w, domain.Validation("readings", "批量读数格式无效"))
				return
			}
		} else {
			var input application.ReadingInput
			if err := json.Unmarshal(raw, &input); err != nil {
				writeError(w, domain.Validation("body", "读数格式无效"))
				return
			}
			inputs = []application.ReadingInput{input}
		}
		result, err := s.app.SubmitReadings(r.Context(), batchID, inputs, mutation)
		writeResult(w, http.StatusOK, result, err)
	case "deviations":
		if len(parts) == 3 && r.Method == http.MethodPatch {
			var input application.ResolveDeviationInput
			if err := decodeJSON(r, &input); err != nil {
				writeError(w, err)
				return
			}
			result, err := s.app.ResolveDeviation(r.Context(), batchID, parts[2], input, mutation)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	case "review":
		if len(parts) == 2 && r.Method == http.MethodPost {
			result, err := s.app.SubmitReview(r.Context(), batchID, mutation)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		if len(parts) == 3 && parts[2] == "decision" && r.Method == http.MethodPost {
			var input application.ReviewDecisionInput
			if err := decodeJSON(r, &input); err != nil {
				writeError(w, err)
				return
			}
			result, err := s.app.DecideReview(r.Context(), batchID, input, mutation)
			writeResult(w, http.StatusOK, result, err)
			return
		}
		methodNotAllowed(w, http.MethodPost)
	case "correction":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.CompleteCorrection(r.Context(), batchID, mutation)
		writeResult(w, http.StatusOK, result, err)
	case "credential":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var input struct {
			ReviewerName string `json:"reviewerName"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.app.IssueCredential(r.Context(), batchID, input.ReviewerName, mutation)
		writeResult(w, http.StatusCreated, result, err)
	default:
		writeError(w, domain.NotFound("路径", r.URL.Path))
	}
}

func (s *Server) credentialResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/admission-credentials/"), "/")
	if id == "" {
		writeError(w, domain.NotFound("准入凭据", ""))
		return
	}
	if r.URL.Query().Get("verify") == "true" {
		section := r.URL.Query().Get("evidenceSection")
		if section == "" {
			section = r.URL.Query().Get("section")
		}
		result, err := s.app.VerifyCredential(r.Context(), id, section)
		writeResult(w, http.StatusOK, result, err)
		return
	}
	credential, err := s.app.GetCredential(r.Context(), id)
	writeResult(w, http.StatusOK, credential, err)
}

func parseBatchQuery(r *http.Request) (application.BatchListQuery, error) {
	q := r.URL.Query()
	result := application.BatchListQuery{Status: domain.BatchStatus(q.Get("status")), VenueID: q.Get("venueId"), OwnerName: q.Get("ownerName"), Cursor: q.Get("cursor")}
	var err error
	fromValue := q.Get("plannedStartFrom")
	if fromValue == "" {
		fromValue = q.Get("plannedStartAtFrom")
	}
	if value := fromValue; value != "" {
		t, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return result, domain.Validation("plannedStartFrom", "必须是 RFC3339 时间")
		}
		result.PlannedStartFrom = &t
	}
	toValue := q.Get("plannedStartTo")
	if toValue == "" {
		toValue = q.Get("plannedStartAtTo")
	}
	if value := toValue; value != "" {
		t, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return result, domain.Validation("plannedStartTo", "必须是 RFC3339 时间")
		}
		result.PlannedStartTo = &t
	}
	if value := q.Get("limit"); value != "" {
		result.Limit, err = strconv.Atoi(value)
		if err != nil {
			return result, domain.Validation("limit", "必须是整数")
		}
	}
	return result, nil
}
func parseReadingQuery(r *http.Request) (application.ReadingQuery, error) {
	q := r.URL.Query()
	result := application.ReadingQuery{StageID: q.Get("stageId"), Verdict: q.Get("verdict"), Cursor: q.Get("cursor")}
	var err error
	if value := q.Get("attempt"); value != "" {
		result.Attempt, err = strconv.Atoi(value)
		if err != nil {
			return result, domain.Validation("attempt", "必须是整数")
		}
	}
	if value := q.Get("limit"); value != "" {
		result.Limit, err = strconv.Atoi(value)
		if err != nil {
			return result, domain.Validation("limit", "必须是整数")
		}
	}
	if value := q.Get("from"); value != "" {
		t, e := time.Parse(time.RFC3339, value)
		if e != nil {
			return result, domain.Validation("from", "必须是 RFC3339 时间")
		}
		result.From = &t
	}
	if value := q.Get("to"); value != "" {
		t, e := time.Parse(time.RFC3339, value)
		if e != nil {
			return result, domain.Validation("to", "必须是 RFC3339 时间")
		}
		result.To = &t
	}
	return result, nil
}

func parseMutation(r *http.Request) (application.Mutation, error) {
	value := r.Header.Get("If-Match-Version")
	if value == "" {
		value = r.URL.Query().Get("expectedVersion")
	}
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return application.Mutation{}, domain.Validation("expectedVersion", "必须通过 If-Match-Version 或 expectedVersion 提供整数版本")
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return application.Mutation{}, domain.Validation("Idempotency-Key", "写操作必须提供幂等请求键")
	}
	return application.Mutation{ExpectedVersion: version, IdempotencyKey: key}, nil
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return domain.Validation("body", "请求体不能为空")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Validation("body", "请求体必须是合法 JSON")
	}
	return nil
}

func writeResult(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := domain.ErrorCode("internal_error")
	message := err.Error()
	field := ""
	var dErr *domain.Error
	if errors.As(err, &dErr) {
		code, message, field = dErr.Code, dErr.Message, dErr.Field
		switch dErr.Code {
		case domain.CodeValidation:
			status = http.StatusBadRequest
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeConflict:
			status = http.StatusConflict
		case domain.CodeState:
			status = http.StatusUnprocessableEntity
		case domain.CodeIntegrity:
			status = http.StatusInternalServerError
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "field": field}})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, domain.InvalidState(fmt.Sprintf("仅支持 %s 请求", strings.Join(methods, ", "))))
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}
