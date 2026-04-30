package units

import (
	"math"
	"testing"
)

func TestNormalizeDPI(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero means missing", 0, DefaultDPI},
		{"negative treated as missing", -50, DefaultDPI},
		{"low raised to default", 72, DefaultDPI},
		{"equal to default kept", DefaultDPI, DefaultDPI},
		{"high kept as-is", 600, 600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeDPI(c.in); got != c.want {
				t.Fatalf("NormalizeDPI(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestMmToPxAt300DPI(t *testing.T) {
	// A4 短边 210mm 在 300DPI 下约 2480px。
	if got := MmToPx(210, 300); got != 2480 {
		t.Fatalf("MmToPx(210, 300) = %d, want 2480", got)
	}
	// 单英寸 = 25.4mm 在 300DPI 下应为 300px。
	if got := MmToPx(InchMM, 300); got != 300 {
		t.Fatalf("MmToPx(25.4, 300) = %d, want 300", got)
	}
}

func TestCmToPx(t *testing.T) {
	// 10cm 在 300DPI 下 = MmToPx(100) = round(100*300/25.4) = 1181。
	if got := CmToPx(10, 300); got != 1181 {
		t.Fatalf("CmToPx(10, 300) = %d, want 1181", got)
	}
}

func TestInchToPx(t *testing.T) {
	if got := InchToPx(2, 300); got != 600 {
		t.Fatalf("InchToPx(2, 300) = %d, want 600", got)
	}
}

func TestRoundTripMmPx(t *testing.T) {
	// 像素 → 毫米 → 像素应该基本守恒（允许 1 像素偏差，因四舍五入）。
	for _, px := range []int{1, 300, 2480, 12345} {
		mm := PxToMm(px, 300)
		got := MmToPx(mm, 300)
		if diff := abs(got - px); diff > 1 {
			t.Fatalf("round trip px %d -> mm %.4f -> px %d, diff %d > 1", px, mm, got, diff)
		}
	}
}

func TestPxToCmAndInch(t *testing.T) {
	if got := PxToCm(1181, 300); math.Abs(got-10) > 0.01 {
		t.Fatalf("PxToCm(1181, 300) = %.4f, want ≈ 10", got)
	}
	if got := PxToInch(300, 300); math.Abs(got-1) > 1e-6 {
		t.Fatalf("PxToInch(300, 300) = %.6f, want 1", got)
	}
}

func TestDPIFallback(t *testing.T) {
	// DPI 0 应自动回退到 300。
	if MmToPx(100, 0) != MmToPx(100, 300) {
		t.Fatalf("dpi=0 fallback not applied")
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
