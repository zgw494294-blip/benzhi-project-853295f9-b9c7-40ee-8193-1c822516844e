package application

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type Service struct {
	store          BatchRepository
	planner        StagePlanner
	evaluator      ReadingEvaluator
	clock          Clock
	newID          func(string) string
	readingCacheMu sync.RWMutex
	readingCache   map[readingQueryCacheKey]ReadingQueryResult
}

func NewService(repository BatchRepository, planner StagePlanner, evaluator ReadingEvaluator, clock Clock, newID func(string) string) *Service {
	if clock == nil {
		clock = SystemClock{}
	}
	if newID == nil {
		newID = NewID
	}
	return &Service{store: repository, planner: planner, evaluator: evaluator, clock: clock, newID: newID, readingCache: make(map[readingQueryCacheKey]ReadingQueryResult)}
}

func NewID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("生成随机标识失败: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

func payloadHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validateMutation(m Mutation) error {
	if m.ExpectedVersion < 1 {
		return fmt.Errorf("expectedVersion 必须大于 0")
	}
	if strings.TrimSpace(m.IdempotencyKey) == "" {
		return fmt.Errorf("Idempotency-Key 不能为空")
	}
	if len(m.IdempotencyKey) > 128 {
		return fmt.Errorf("Idempotency-Key 不能超过 128 字节")
	}
	return nil
}
