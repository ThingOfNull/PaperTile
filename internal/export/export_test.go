package export

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-printtile-pro/internal/imaging"

	xi "github.com/disintegration/imaging"
)

// newTestImage 构造一张用于单测的 LoadedImage：1000×1000 纯色、DPI 300。
func newTestImage(w, h int) *imaging.LoadedImage {
	img := xi.New(w, h, color.RGBA{R: 120, G: 180, B: 220, A: 255})
	return &imaging.LoadedImage{
		Image:  img,
		Width:  w,
		Height: h,
		Format: "png",
		DPIX:   300,
		DPIY:   300,
	}
}

func TestRunEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "e2e.pdf")
	stages := make(map[Stage]int)
	req := Request{
		Source:    newTestImage(1500, 2000),
		TargetWMm: 297, // A3 宽
		TargetHMm: 420, // A3 高 → 源图会被 LANCZOS 放大到 A3 对应像素
		PaperWMm:  210, // A4
		PaperHMm:  297,
		MarginMm:  5,
		OverlapMm: 3,
		Output:    out,
		Progress: func(s Stage, p int) {
			stages[s] = p
		},
	}
	res, err := Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pages < 2 {
		t.Fatalf("expected at least 2 pages for A3 on A4, got %d", res.Pages)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output is not a valid PDF")
	}
	// 应至少见过 encoding / completed 两个阶段。
	if _, ok := stages[StageEncoding]; !ok {
		t.Fatalf("missing encoding progress")
	}
	if stages[StageCompleted] != 100 {
		t.Fatalf("completed stage percent = %d, want 100", stages[StageCompleted])
	}
}

func TestRunWithSkip(t *testing.T) {
	// 2000×2000 源图缩放到 297×420 mm（A3）切到 A4，预期出若干块。
	// 跳过所有偶数行：最终页数应等于"非偶数行的 tile 总数"。
	out := filepath.Join(t.TempDir(), "skip.pdf")
	var rowHits, colHits []int
	req := Request{
		Source:    newTestImage(2000, 2000),
		TargetWMm: 297,
		TargetHMm: 420,
		PaperWMm:  210,
		PaperHMm:  297,
		MarginMm:  5,
		OverlapMm: 0,
		Output:    out,
		Skip: func(col, row int) bool {
			rowHits = append(rowHits, row)
			colHits = append(colHits, col)
			return row%2 == 0
		},
	}
	res, err := Run(req)
	if err != nil {
		t.Fatalf("Run with skip: %v", err)
	}
	if len(rowHits) == 0 {
		t.Fatalf("Skip predicate never invoked")
	}
	// 结果页数应严格小于 Skip 被调用的次数（= 总 tile 数）。
	if res.Pages >= len(rowHits) {
		t.Fatalf("expected skipped pages to be fewer than total, got pages=%d total=%d", res.Pages, len(rowHits))
	}
}

func TestRunAllSkipped(t *testing.T) {
	req := Request{
		Source:    newTestImage(500, 500),
		TargetWMm: 210,
		TargetHMm: 297,
		PaperWMm:  210,
		PaperHMm:  297,
		MarginMm:  5,
		Output:    filepath.Join(t.TempDir(), "empty.pdf"),
		Skip:      func(col, row int) bool { return true },
	}
	if _, err := Run(req); err == nil {
		t.Fatalf("expected error when all tiles skipped")
	}
}

func TestRunWithCrop(t *testing.T) {
	out := filepath.Join(t.TempDir(), "crop.pdf")
	req := Request{
		Source:    newTestImage(2000, 2000),
		Crop:      &Rect{X0: 500, Y0: 500, X1: 1500, Y1: 1500},
		TargetWMm: 210,
		TargetHMm: 210,
		PaperWMm:  210,
		PaperHMm:  297,
		MarginMm:  5,
		OverlapMm: 0,
		Output:    out,
	}
	if _, err := Run(req); err != nil {
		t.Fatalf("Run with crop: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output missing: %v", err)
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	good := Request{
		Source:    newTestImage(500, 500),
		TargetWMm: 210,
		TargetHMm: 297,
		PaperWMm:  210,
		PaperHMm:  297,
		MarginMm:  5,
		Output:    filepath.Join(t.TempDir(), "bad.pdf"),
	}
	cases := []struct {
		name string
		mut  func(r *Request)
	}{
		{"no source", func(r *Request) { r.Source = nil }},
		{"target zero", func(r *Request) { r.TargetWMm = 0 }},
		{"paper negative", func(r *Request) { r.PaperHMm = -1 }},
		{"empty output", func(r *Request) { r.Output = "" }},
		{"bad crop", func(r *Request) { r.Crop = &Rect{X0: 10, Y0: 10, X1: 5, Y1: 5} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := good
			c.mut(&r)
			if _, err := Run(r); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

func TestRunSavingModeReducesOrEqualsPages(t *testing.T) {
	// 同一份输入跑两次：标准模式 vs 省纸模式；
	// 省纸模式纸张利用率 ≥ 标准模式，故页数应 ≤ 标准模式。
	src := newTestImage(2000, 2000)
	baseReq := Request{
		Source:    src,
		TargetWMm: 297,
		TargetHMm: 420, // A3
		PaperWMm:  210,
		PaperHMm:  297, // A4
		MarginMm:  5,
	}

	stdOut := filepath.Join(t.TempDir(), "std.pdf")
	std := baseReq
	std.Output = stdOut
	std.Mode = ModeStandard
	stdRes, err := Run(std)
	if err != nil {
		t.Fatalf("standard: %v", err)
	}

	savOut := filepath.Join(t.TempDir(), "sav.pdf")
	sav := baseReq
	sav.Output = savOut
	sav.Mode = ModeSaving
	sav.AllowRotate = true
	savRes, err := Run(sav)
	if err != nil {
		t.Fatalf("saving: %v", err)
	}

	if savRes.Pages > stdRes.Pages {
		t.Fatalf("saving mode uses MORE pages (%d) than standard (%d)",
			savRes.Pages, stdRes.Pages)
	}

	data, err := os.ReadFile(savOut)
	if err != nil {
		t.Fatalf("read saving pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("saving output not a valid PDF")
	}
}

func TestRunSavingModeWithLandscapeInput(t *testing.T) {
	// 横向目标尺寸 + A4 纸。省纸模式应选 landscape 页面装箱，单页能装下更多块。
	req := Request{
		Source:      newTestImage(2000, 1000),
		TargetWMm:   420, // 横向
		TargetHMm:   210,
		PaperWMm:    210,
		PaperHMm:    297,
		MarginMm:    5,
		Mode:        ModeSaving,
		AllowRotate: true,
		Output:      filepath.Join(t.TempDir(), "sav_land.pdf"),
	}
	res, err := Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Pages < 1 {
		t.Fatalf("expected at least one page")
	}
}

func TestDefaultFileName(t *testing.T) {
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		in, out string
	}{
		{`C:\photos\sunset.jpg`, "sunset_20260417.pdf"},
		{`/home/u/foo.png`, "foo_20260417.pdf"},
		{`no_ext`, "no_ext_20260417.pdf"},
		{``, "image_20260417.pdf"},
	}
	for _, c := range cases {
		got := DefaultFileName(c.in, now)
		if got != c.out {
			t.Errorf("DefaultFileName(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}
