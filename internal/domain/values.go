package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

func (r Range) Validate(field string, absoluteMin, absoluteMax float64) error {
	if math.IsNaN(r.Min) || math.IsNaN(r.Max) || math.IsInf(r.Min, 0) || math.IsInf(r.Max, 0) {
		return Validation(field, "区间必须使用有限数值")
	}
	if r.Min >= r.Max {
		return Validation(field, "区间下限必须小于上限")
	}
	if r.Min < absoluteMin || r.Max > absoluteMax {
		return Validation(field, fmt.Sprintf("区间必须位于 %.1f 到 %.1f 之间", absoluteMin, absoluteMax))
	}
	return nil
}

func (r Range) Contains(value float64) bool { return value >= r.Min && value <= r.Max }
func (r Range) Width() float64              { return r.Max - r.Min }
func (r Range) Center() float64             { return (r.Min + r.Max) / 2 }

type Sensitivity string

const (
	SensitivityLow      Sensitivity = "low"
	SensitivityMedium   Sensitivity = "medium"
	SensitivityHigh     Sensitivity = "high"
	SensitivityCritical Sensitivity = "critical"
)

func (s Sensitivity) Rank() int {
	switch s {
	case SensitivityLow:
		return 1
	case SensitivityMedium:
		return 2
	case SensitivityHigh:
		return 3
	case SensitivityCritical:
		return 4
	default:
		return 0
	}
}

func (s Sensitivity) Validate() error {
	if s.Rank() == 0 {
		return Validation("sensitivityLevel", "敏感性等级必须是 low、medium、high 或 critical")
	}
	return nil
}

func ValidateName(field, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return Validation(field, "不能为空")
	}
	if len([]rune(value)) > max {
		return Validation(field, fmt.Sprintf("长度不能超过 %d 个字符", max))
	}
	return nil
}

func ValidateTimestamp(field string, value time.Time) error {
	if value.IsZero() {
		return Validation(field, "必须提供有效时间")
	}
	return nil
}
