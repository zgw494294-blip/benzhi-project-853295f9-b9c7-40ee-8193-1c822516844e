package policy

import (
	"fmt"
	"math"
	"sync"
	"time"

	"collection-acclimatization-pass/internal/domain"
)

type IDGenerator func(prefix string) string

type Planner struct {
	NewID IDGenerator

	mu           sync.Mutex
	cachedKey    string
	cachedStages []domain.AcclimatizationStage
	cachedBasis  PlanBasis
}

type PlanBasis = domain.PlanBasis

func (p *Planner) Generate(batch *domain.AcclimatizationBatch) ([]domain.AcclimatizationStage, PlanBasis, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(batch.Profiles) == 0 {
		return nil, PlanBasis{}, domain.InvalidState("生成方案前必须登记展品")
	}
	if p.NewID == nil {
		return nil, PlanBasis{}, domain.Integrity("规划器缺少 ID 生成器")
	}
	basis := PlanBasis{Sensitivity: domain.SensitivityLow, MaxTemperatureRate: math.MaxFloat64, MaxHumidityRate: math.MaxFloat64}
	allowedTemp := domain.Range{Min: -10, Max: 40}
	allowedHumidity := domain.Range{Min: 10, Max: 80}
	for _, profile := range batch.Profiles {
		if profile.SensitivityLevel.Rank() > basis.Sensitivity.Rank() {
			basis.Sensitivity = profile.SensitivityLevel
		}
		basis.MaxTemperatureRate = math.Min(basis.MaxTemperatureRate, profile.MaxTemperatureRate)
		basis.MaxHumidityRate = math.Min(basis.MaxHumidityRate, profile.MaxHumidityRate)
		allowedTemp.Min = math.Max(allowedTemp.Min, profile.TargetTemperatureRange.Min)
		allowedTemp.Max = math.Min(allowedTemp.Max, profile.TargetTemperatureRange.Max)
		allowedHumidity.Min = math.Max(allowedHumidity.Min, profile.TargetHumidityRange.Min)
		allowedHumidity.Max = math.Min(allowedHumidity.Max, profile.TargetHumidityRange.Max)
	}
	if allowedTemp.Min >= allowedTemp.Max || allowedHumidity.Min >= allowedHumidity.Max {
		return nil, basis, domain.Validation("profiles", "展品档案不存在共同安全环境区间")
	}
	cacheKey := fmt.Sprintf("%s:%.6f:%.6f:%.6f:%.6f:%.6f:%.6f", basis.Sensitivity, basis.MaxTemperatureRate, basis.MaxHumidityRate, allowedTemp.Min, allowedTemp.Max, allowedHumidity.Min, allowedHumidity.Max)
	if p.cachedKey == cacheKey {
		return append([]domain.AcclimatizationStage(nil), p.cachedStages...), p.cachedBasis, nil
	}
	tempPads := []float64{4, 2, 0}
	humidityPads := []float64{12, 6, 0}
	baseMinutes := map[domain.Sensitivity]int{domain.SensitivityLow: 30, domain.SensitivityMedium: 45, domain.SensitivityHigh: 60, domain.SensitivityCritical: 90}[basis.Sensitivity]
	stages := make([]domain.AcclimatizationStage, 0, 3)
	for i := 0; i < 3; i++ {
		temp := intersect(domain.Range{Min: batch.VenueTemperatureRange.Min - tempPads[i], Max: batch.VenueTemperatureRange.Max + tempPads[i]}, allowedTemp)
		humidity := intersect(domain.Range{Min: batch.VenueHumidityRange.Min - humidityPads[i], Max: batch.VenueHumidityRange.Max + humidityPads[i]}, allowedHumidity)
		duration := time.Duration(baseMinutes+i*15) * time.Minute
		window := time.Duration(10+i*5) * time.Minute
		stage := domain.AcclimatizationStage{ID: p.NewID("stage"), BatchID: batch.ID, Sequence: i + 1, TargetTemperatureRange: temp, TargetHumidityRange: humidity, MinimumDuration: duration, StabilityWindow: window, Status: domain.StagePlanned, Attempt: 1, Rationale: rationale(i+1, basis)}
		if err := stage.Validate(); err != nil {
			return nil, basis, err
		}
		stages = append(stages, stage)
	}
	p.cachedKey = cacheKey
	p.cachedStages = append([]domain.AcclimatizationStage(nil), stages...)
	p.cachedBasis = basis
	return stages, basis, nil
}

func intersect(a, b domain.Range) domain.Range {
	return domain.Range{Min: math.Max(a.Min, b.Min), Max: math.Min(a.Max, b.Max)}
}

func rationale(sequence int, basis PlanBasis) string {
	labels := []string{"缓冲过渡", "接近展厅", "目标稳定"}
	return fmt.Sprintf("%s阶段：按 %s 敏感性及 %.2f °C/h、%.2f %%RH/h 的最严格变化率控制", labels[sequence-1], basis.Sensitivity, basis.MaxTemperatureRate, basis.MaxHumidityRate)
}
