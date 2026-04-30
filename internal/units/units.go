// Package units 提供物理单位（cm、mm、inch）与像素之间的换算，以及 DPI 规范化。
// 全项目所有 mm ↔ px 的换算必须通过本包函数，禁止在业务代码中散写 25.4、2.54、300 等字面量。
package units

import "math"

// 基础常量。
const (
	// InchMM 1 英寸对应的毫米数（国际标准值）。
	InchMM = 25.4
	// InchCM 1 英寸对应的厘米数。
	InchCM = 2.54
	// DefaultDPI 默认渲染 DPI。原图元数据缺失或低于该值时一律按此处理。
	DefaultDPI = 300
)

// MmToPx 将毫米长度按指定 DPI 换算为像素数，结果四舍五入取整。
// 当 dpi <= 0 时视为 DefaultDPI。
func MmToPx(mm float64, dpi int) int {
	d := float64(NormalizeDPI(dpi))
	return int(math.Round(mm * d / InchMM))
}

// CmToPx 将厘米长度按指定 DPI 换算为像素数，结果四舍五入取整。
func CmToPx(cm float64, dpi int) int {
	return MmToPx(cm*10, dpi)
}

// InchToPx 将英寸长度按指定 DPI 换算为像素数，结果四舍五入取整。
func InchToPx(inch float64, dpi int) int {
	d := float64(NormalizeDPI(dpi))
	return int(math.Round(inch * d))
}

// PxToMm 将像素数按指定 DPI 换算为毫米长度。
func PxToMm(px int, dpi int) float64 {
	d := float64(NormalizeDPI(dpi))
	return float64(px) * InchMM / d
}

// PxToCm 将像素数按指定 DPI 换算为厘米长度。
func PxToCm(px int, dpi int) float64 {
	return PxToMm(px, dpi) / 10
}

// PxToInch 将像素数按指定 DPI 换算为英寸长度。
func PxToInch(px int, dpi int) float64 {
	d := float64(NormalizeDPI(dpi))
	return float64(px) / d
}

// NormalizeDPI 返回生效 DPI：原值小于 DefaultDPI（含 0、负值）则提升为 DefaultDPI，否则保留。
// 该函数实现 PRD 中"缺失或 <300 → 按 300 处理；≥300 → 保留原值"的约束。
func NormalizeDPI(raw int) int {
	if raw < DefaultDPI {
		return DefaultDPI
	}
	return raw
}
