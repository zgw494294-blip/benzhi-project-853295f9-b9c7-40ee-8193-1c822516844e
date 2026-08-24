package domain

import (
	"math"
	"sort"
	"strings"
)

type ObjectMaterialProfile struct {
	ID                     string      `json:"id"`
	BatchID                string      `json:"batchId"`
	CollectionCode         string      `json:"collectionCode"`
	MaterialClasses        []string    `json:"materialClasses"`
	SensitivityLevel       Sensitivity `json:"sensitivityLevel"`
	TargetTemperatureRange Range       `json:"targetTemperatureRange"`
	TargetHumidityRange    Range       `json:"targetHumidityRange"`
	MaxTemperatureRate     float64     `json:"maxTemperatureRate"`
	MaxHumidityRate        float64     `json:"maxHumidityRate"`
}

func ValidateProfileIntersection(profiles []ObjectMaterialProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	tMin, tMax := -math.MaxFloat64, math.MaxFloat64
	hMin, hMax := -math.MaxFloat64, math.MaxFloat64
	for _, p := range profiles {
		tMin = math.Max(tMin, p.TargetTemperatureRange.Min)
		tMax = math.Min(tMax, p.TargetTemperatureRange.Max)
		hMin = math.Max(hMin, p.TargetHumidityRange.Min)
		hMax = math.Min(hMax, p.TargetHumidityRange.Max)
	}
	if tMin >= tMax {
		return Validation("profiles", "全部材料档案不存在共同安全温度区间")
	}
	if hMin >= hMax {
		return Validation("profiles", "全部材料档案不存在共同安全湿度区间")
	}
	return nil
}

func NewProfile(p ObjectMaterialProfile, venueTemp, venueHumidity Range) (ObjectMaterialProfile, error) {
	if err := ValidateName("collectionCode", p.CollectionCode, 80); err != nil {
		return p, err
	}
	if len(p.MaterialClasses) == 0 {
		return p, Validation("materialClasses", "至少登记一种材料")
	}
	seen := make(map[string]struct{}, len(p.MaterialClasses))
	materials := make([]string, 0, len(p.MaterialClasses))
	for _, material := range p.MaterialClasses {
		material = strings.TrimSpace(material)
		if material == "" {
			return p, Validation("materialClasses", "材料名称不能为空")
		}
		if _, exists := seen[material]; !exists {
			seen[material] = struct{}{}
			materials = append(materials, material)
		}
	}
	sort.Strings(materials)
	p.MaterialClasses = materials
	if err := p.SensitivityLevel.Validate(); err != nil {
		return p, err
	}
	if err := p.TargetTemperatureRange.Validate("targetTemperatureRange", -10, 40); err != nil {
		return p, err
	}
	if err := p.TargetHumidityRange.Validate("targetHumidityRange", 10, 80); err != nil {
		return p, err
	}
	if p.TargetTemperatureRange.Min > venueTemp.Min || p.TargetTemperatureRange.Max < venueTemp.Max {
		return p, Validation("targetTemperatureRange", "展品温度边界必须完整覆盖展厅目标边界")
	}
	if p.TargetHumidityRange.Min > venueHumidity.Min || p.TargetHumidityRange.Max < venueHumidity.Max {
		return p, Validation("targetHumidityRange", "展品湿度边界必须完整覆盖展厅目标边界")
	}
	if p.MaxTemperatureRate <= 0 || p.MaxTemperatureRate > 5 {
		return p, Validation("maxTemperatureRate", "温度变化率必须大于 0 且不超过 5 °C/h")
	}
	if p.MaxHumidityRate <= 0 || p.MaxHumidityRate > 20 {
		return p, Validation("maxHumidityRate", "湿度变化率必须大于 0 且不超过 20 %RH/h")
	}
	limits := map[Sensitivity][2]float64{
		SensitivityLow: {5, 20}, SensitivityMedium: {3, 12},
		SensitivityHigh: {2, 8}, SensitivityCritical: {1, 5},
	}
	limit := limits[p.SensitivityLevel]
	if p.MaxTemperatureRate > limit[0] || p.MaxHumidityRate > limit[1] {
		return p, Validation("sensitivityLevel", "允许变化率与材料敏感性保护阈值矛盾")
	}
	return p, nil
}
