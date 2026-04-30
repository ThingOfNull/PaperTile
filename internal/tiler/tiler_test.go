package tiler

import (
	"errors"
	"testing"

	"go-printtile-pro/internal/units"
)

// a4 返回 A4 纸张的 Params 预设（边距 5mm、无重叠、300DPI、指定源图尺寸）。
func a4(w, h int) Params {
	return Params{
		SourceW:   w,
		SourceH:   h,
		PaperWMm:  210,
		PaperHMm:  297,
		MarginMm:  5,
		OverlapMm: 0,
		DPI:       300,
	}
}

func TestBuildPlanSingleTile(t *testing.T) {
	// A4 打印区约为 200 × 287 mm，对应 300DPI 下 ~2362 × 3390 px。
	// 源图 1000×1000 完全塞进一页。
	plan, err := BuildPlan(a4(1000, 1000))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Cols != 1 || plan.Rows != 1 || len(plan.Tiles) != 1 {
		t.Fatalf("expected 1x1 grid, got %dx%d (%d tiles)", plan.Cols, plan.Rows, len(plan.Tiles))
	}
	tile := plan.Tiles[0]
	if tile.X0 != 0 || tile.Y0 != 0 || tile.X1 != 1000 || tile.Y1 != 1000 {
		t.Fatalf("tile rect = (%d,%d,%d,%d), want (0,0,1000,1000)",
			tile.X0, tile.Y0, tile.X1, tile.Y1)
	}
}

func TestBuildPlanExactMultiple(t *testing.T) {
	// 用"刚好能被 tile 尺寸整除"的源图验证网格数量。
	p := a4(1, 1)
	tile := units.MmToPx(200, 300) // effectiveW = 210-10 = 200mm → ~2362px
	p.SourceW = tile * 3
	p.SourceH = units.MmToPx(287, 300) // = effectiveH in pixels
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Cols != 3 || plan.Rows != 1 {
		t.Fatalf("grid = %dx%d, want 3x1", plan.Cols, plan.Rows)
	}
	// 第三块右边界应 = 源图宽。
	last := plan.Tiles[2]
	if last.X1 != p.SourceW {
		t.Fatalf("last tile X1 = %d, want %d", last.X1, p.SourceW)
	}
}

func TestBuildPlanWithOverlap(t *testing.T) {
	// 重叠 10mm → 相邻块起点差 = (200 - 10) mm 的像素值。
	p := a4(5000, 1000)
	p.OverlapMm = 10
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	expectedStep := units.MmToPx(200-10, 300)
	if plan.StepPxX != expectedStep {
		t.Fatalf("StepPxX = %d, want %d", plan.StepPxX, expectedStep)
	}
	expectedOverlapPx := units.MmToPx(10, 300)
	if plan.OverlapPxX != expectedOverlapPx {
		t.Fatalf("OverlapPxX = %d, want %d", plan.OverlapPxX, expectedOverlapPx)
	}
	// 相邻两列应有重叠：tile[1].X0 < tile[0].X1。
	if len(plan.Tiles) < 2 {
		t.Fatalf("expected at least 2 tiles, got %d", len(plan.Tiles))
	}
	if plan.Tiles[1].X0 >= plan.Tiles[0].X1 {
		t.Fatalf("tiles not overlapping: tile0=%v tile1=%v", plan.Tiles[0], plan.Tiles[1])
	}
	// 最后一块必须覆盖到源图右边界。
	var maxX1 int
	for _, ti := range plan.Tiles {
		if ti.X1 > maxX1 {
			maxX1 = ti.X1
		}
	}
	if maxX1 != p.SourceW {
		t.Fatalf("last tile doesn't reach source edge: got X1=%d, want %d", maxX1, p.SourceW)
	}
}

func TestBuildPlanLastTileClamped(t *testing.T) {
	// 源图右边不满一块 → 最后一列被 clamp 到源图宽。
	p := a4(2500, 500)
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Cols < 2 {
		t.Fatalf("expected >=2 cols, got %d", plan.Cols)
	}
	last := plan.Tiles[len(plan.Tiles)-1]
	if last.X1 != p.SourceW {
		t.Fatalf("last tile X1 = %d, want %d", last.X1, p.SourceW)
	}
	if last.Width() >= plan.TilePxW {
		t.Fatalf("last tile should be narrower than full tile, got %d vs %d", last.Width(), plan.TilePxW)
	}
}

func TestBuildPlanInvalidParams(t *testing.T) {
	if _, err := BuildPlan(Params{SourceW: 0, SourceH: 100, PaperWMm: 210, PaperHMm: 297, DPI: 300}); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("expected ErrInvalidSize, got %v", err)
	}
	if _, err := BuildPlan(Params{SourceW: 100, SourceH: 100, PaperWMm: 210, PaperHMm: 297, DPI: 0}); !errors.Is(err, ErrInvalidDPI) {
		t.Fatalf("expected ErrInvalidDPI, got %v", err)
	}
	if _, err := BuildPlan(Params{SourceW: 100, SourceH: 100, PaperWMm: 210, PaperHMm: 297, MarginMm: 200, DPI: 300}); !errors.Is(err, ErrMarginTooLarge) {
		t.Fatalf("expected ErrMarginTooLarge, got %v", err)
	}
	if _, err := BuildPlan(Params{SourceW: 100, SourceH: 100, PaperWMm: 210, PaperHMm: 297, MarginMm: 5, OverlapMm: 250, DPI: 300}); !errors.Is(err, ErrOverlapOutOfRange) {
		t.Fatalf("expected ErrOverlapOutOfRange, got %v", err)
	}
}

func TestComputeCount(t *testing.T) {
	cases := []struct {
		total, tile, step, want int
	}{
		{1000, 1000, 900, 1},
		{999, 1000, 900, 1},
		{1900, 1000, 900, 2},
		{2000, 1000, 900, 3},
		{1800, 1000, 900, 2},
		{4500, 1000, 1000, 5},
	}
	for _, c := range cases {
		if got := computeCount(c.total, c.tile, c.step); got != c.want {
			t.Errorf("computeCount(total=%d, tile=%d, step=%d) = %d, want %d",
				c.total, c.tile, c.step, got, c.want)
		}
	}
}

func TestTilesCoverEntireSource(t *testing.T) {
	// 验证切块完整覆盖源图（每一点至少属于一个块）。
	p := a4(7000, 9000)
	p.OverlapMm = 8
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	// 简化覆盖检查：检查若干代表点（四角 + 中心）。
	points := [][2]int{{0, 0}, {p.SourceW - 1, 0}, {0, p.SourceH - 1}, {p.SourceW - 1, p.SourceH - 1}, {p.SourceW / 2, p.SourceH / 2}}
	for _, pt := range points {
		covered := false
		for _, tl := range plan.Tiles {
			if pt[0] >= tl.X0 && pt[0] < tl.X1 && pt[1] >= tl.Y0 && pt[1] < tl.Y1 {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("point %v not covered by any tile", pt)
		}
	}
}
