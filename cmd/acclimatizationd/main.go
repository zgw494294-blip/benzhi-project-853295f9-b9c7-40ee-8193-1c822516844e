package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"collection-acclimatization-pass/internal/application"
	"collection-acclimatization-pass/internal/domain"
	"collection-acclimatization-pass/internal/httpapi"
	"collection-acclimatization-pass/internal/policy"
	"collection-acclimatization-pass/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19127", "回环监听地址")
	selfcheck := flag.Bool("selfcheck", false, "执行完整回环自检")
	dbPath := flag.String("db", filepath.Join(os.TempDir(), "collection-acclimatization-pass.json"), "持久化文件")
	flag.Parse()
	resolved, err := resolveAddr(*addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *selfcheck {
		if err = runSelfcheck(resolved); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err = runServer(resolved, *dbPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveAddr(value string) (string, error) {
	if port := os.Getenv("PORT"); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1024 || n > 65535 {
			return "", fmt.Errorf("PORT 必须是 1024-65535 的端口号")
		}
		value = "127.0.0.1:" + port
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("监听地址无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "[::1]" {
		return "", fmt.Errorf("监听地址必须是回环地址")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return "", fmt.Errorf("端口必须是 1024-65535")
	}
	return value, nil
}

func buildApp(dbPath string) (*application.Service, *store.SQLiteStore, error) {
	repository, err := store.Open(context.Background(), dbPath)
	if err != nil {
		return nil, nil, err
	}
	planner := policy.Planner{NewID: application.NewID}
	evaluator := &policy.Evaluator{MaxGap: 15 * time.Minute}
	app := application.NewService(repository, planner, evaluator, application.SystemClock{}, application.NewID)
	return app, repository, nil
}

func runServer(addr, dbPath string) error {
	app, repository, err := buildApp(dbPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	server := httpapi.New(app)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() { _ = server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func runSelfcheck(addr string) error {
	dbPath := filepath.Join(os.TempDir(), "acclimatization-selfcheck-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".json")
	defer os.Remove(dbPath)
	app, repository, err := buildApp(dbPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	server := httpapi.New(app)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	go server.Serve(listener)
	base := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 3 * time.Second}
	var created struct {
		Batch *domain.AcclimatizationBatch `json:"batch"`
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches", map[string]any{"venueId": "gallery-a", "plannedStartAt": "2026-09-01T09:00:00Z", "ownerName": "林文", "venueTemperatureRange": map[string]float64{"min": 20, "max": 22}, "venueHumidityRange": map[string]float64{"min": 45, "max": 50}}, "create-1", 0, &created); err != nil {
		return fmt.Errorf("创建自检批次: %w", err)
	}
	batch := created.Batch
	profile := map[string]any{"collectionCode": "OBJ-001", "materialClasses": []string{"木", "纸"}, "sensitivityLevel": "medium", "targetTemperatureRange": map[string]float64{"min": 20, "max": 22}, "targetHumidityRange": map[string]float64{"min": 45, "max": 50}, "maxTemperatureRate": 3.0, "maxHumidityRate": 12.0}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/profiles", profile, "profile-1", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/plan", map[string]any{}, "plan-1", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	var plan struct {
		PlanDigest string `json:"planDigest"`
	}
	if err = call(client, http.MethodGet, base+"/v1/acclimatization-batches/"+batch.ID+"/plan/diff", nil, "", 0, &plan); err != nil {
		return err
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/plan/freeze", map[string]any{"planDigest": plan.PlanDigest}, "freeze-1", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/start", map[string]any{}, "start-1", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	for stageIndex := 0; stageIndex < len(batch.Stages); stageIndex++ {
		stage := batch.Stages[stageIndex]
		start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(stageIndex) * 2 * time.Hour)
		for i := 0; i <= int(stage.MinimumDuration/(5*time.Minute)); i++ {
			at := start.Add(time.Duration(i) * 5 * time.Minute)
			reading := map[string]any{"stageId": stage.ID, "observedAt": at.Format(time.RFC3339), "temperature": 21.0, "humidity": 47.0}
			if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/readings", reading, fmt.Sprintf("reading-%d-%d", stageIndex, i), batch.Version, &struct{}{}); err != nil {
				return fmt.Errorf("阶段 %d 读数: %w", stage.Sequence, err)
			}
			batch, err = refresh(client, base, batch.ID)
			if err != nil {
				return err
			}
			if batch.CurrentStageID != stage.ID {
				break
			}
		}
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/review", map[string]any{}, "review-submit", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/review/decision", map[string]any{"reviewerName": "周宁", "decision": "approved", "reason": "温湿度稳定且证据完整"}, "review-decision", batch.Version, &struct{}{}); err != nil {
		return err
	}
	batch, err = refresh(client, base, batch.ID)
	if err != nil {
		return err
	}
	var issued struct {
		Credential *domain.AdmissionCredential `json:"credential"`
	}
	if err = call(client, http.MethodPost, base+"/v1/acclimatization-batches/"+batch.ID+"/credential", map[string]any{"reviewerName": "周宁"}, "credential-issue", batch.Version, &issued); err != nil {
		return err
	}
	if issued.Credential == nil || !strings.HasPrefix(issued.Credential.EvidenceDigest, "sha256:") {
		return fmt.Errorf("自检未得到有效凭据摘要")
	}
	return server.Shutdown(context.Background())
}

func refresh(client *http.Client, base, id string) (*domain.AcclimatizationBatch, error) {
	var result domain.AcclimatizationBatch
	if err := call(client, http.MethodGet, base+"/v1/acclimatization-batches/"+id, nil, "", 0, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func call(client *http.Client, method, url string, body any, key string, version int64, target any) error {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if version > 0 {
		req.Header.Set("If-Match-Version", strconv.FormatInt(version, 10))
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if target != nil && len(data) > 0 {
		if err = json.Unmarshal(data, target); err != nil {
			return err
		}
	}
	return nil
}
