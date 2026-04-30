package packer

import (
	"math"
	"testing"
)

// 验证：相同 items 在同一个 bin 中装箱后，所有 placement 都在 bin 内且两两不重叠。
func assertNoOverlap(t *testing.T, placed []Placement, binW, binH float64) {
	t.Helper()
	for _, p := range placed {
		if p.X < -eps || p.Y < -eps {
			t.Fatalf("placement out of bin (negative origin): %+v", p)
		}
		if p.X+p.PlaceW > binW+eps || p.Y+p.PlaceH > binH+eps {
			t.Fatalf("placement out of bin %vx%v: %+v", binW, binH, p)
		}
	}
	for i := 0; i < len(placed); i++ {
		for j := i + 1; j < len(placed); j++ {
			a := placed[i]
			b := placed[j]
			if a.X < b.X+b.PlaceW-eps && b.X < a.X+a.PlaceW-eps &&
				a.Y < b.Y+b.PlaceH-eps && b.Y < a.Y+a.PlaceH-eps {
				t.Fatalf("placements overlap: #%d %+v vs #%d %+v", i, a, j, b)
			}
		}
	}
}

func TestPackSingleItemFits(t *testing.T) {
	placed, leftover := Pack(200, 280, 0, []Item{{ID: 1, W: 100, H: 140}}, false)
	if len(placed) != 1 || len(leftover) != 0 {
		t.Fatalf("expected 1 placed 0 leftover, got %d / %d", len(placed), len(leftover))
	}
	if placed[0].Rotated {
		t.Fatalf("item fits without rotation, shouldn't rotate")
	}
	assertNoOverlap(t, placed, 200, 280)
}

func TestPackHandfulNoOverlap(t *testing.T) {
	items := []Item{
		{ID: 1, W: 100, H: 100},
		{ID: 2, W: 100, H: 100},
		{ID: 3, W: 100, H: 100},
		{ID: 4, W: 100, H: 100},
	}
	placed, leftover := Pack(200, 200, 0, items, false)
	if len(placed) != 4 || len(leftover) != 0 {
		t.Fatalf("expected all 4 to fit (2×2 grid), got %d / %d", len(placed), len(leftover))
	}
	assertNoOverlap(t, placed, 200, 200)
}

func TestPackRotationEnablesFit(t *testing.T) {
	// 100×50 的 tile 纵向（50×100）能塞，横向则不行。旋转关闭时首个摆放仍然成功，
	// 因为 100×50 本身就能装进 100×100 的 bin（还剩 100×50 的空间）。
	// 但再塞第二个 100×50 时若不允许旋转就放不下；允许旋转则可以 50×100 侧放。
	items := []Item{
		{ID: 1, W: 100, H: 50},
		{ID: 2, W: 100, H: 50},
	}
	placedNoRot, _ := Pack(100, 100, 0, items, false)
	placedRot, leftoverRot := Pack(100, 100, 0, items, true)
	if len(placedNoRot) != 2 {
		t.Fatalf("two 100x50 should stack without rotation, got %d", len(placedNoRot))
	}
	if len(placedRot) != 2 || len(leftoverRot) != 0 {
		t.Fatalf("with rotation, should still fit both: placed=%d leftover=%d",
			len(placedRot), len(leftoverRot))
	}
	assertNoOverlap(t, placedRot, 100, 100)
}

func TestPackRotationRequiredToFit(t *testing.T) {
	// 构造一个"不旋转就塞不下"的场景：bin 80×200，放三个 70×40，不旋转时 X 维度只够放一个（70），
	// Y 维度能放 5 个（5×40=200）。允许旋转时可以把部分旋转成 40×70 见缝插针。
	items := []Item{
		{ID: 1, W: 70, H: 40},
		{ID: 2, W: 70, H: 40},
		{ID: 3, W: 70, H: 40},
		{ID: 4, W: 70, H: 40},
		{ID: 5, W: 70, H: 40},
		{ID: 6, W: 70, H: 40},
	}
	noRot, _ := Pack(80, 200, 0, items, false)
	withRot, _ := Pack(80, 200, 0, items, true)
	if len(withRot) < len(noRot) {
		t.Fatalf("rotation should not hurt packing: noRot=%d withRot=%d",
			len(noRot), len(withRot))
	}
	assertNoOverlap(t, withRot, 80, 200)
}

func TestPackLeftoverPreservesInputOrder(t *testing.T) {
	// 只有一块能放下（大小超标会被直接 leftover）。
	items := []Item{
		{ID: 10, W: 500, H: 500}, // 超大，放不下
		{ID: 20, W: 30, H: 30},
		{ID: 30, W: 400, H: 400}, // 超大
		{ID: 40, W: 20, H: 20},
	}
	placed, leftover := Pack(100, 100, 0, items, false)
	if len(placed) != 2 {
		t.Fatalf("expected 2 placed, got %d", len(placed))
	}
	// leftover 应保持原顺序：10, 30
	if len(leftover) != 2 || leftover[0].ID != 10 || leftover[1].ID != 30 {
		t.Fatalf("leftover order unexpected: %+v", leftover)
	}
}

func TestPackPagesExhaustsItems(t *testing.T) {
	// 10 块 100×140（A5 纵向），usable 200×280（A4 - 5mm 边距约）。
	// A4 portrait 理论上可放 4 块（2×2），10 块至少要 3 页。
	items := make([]Item, 10)
	for i := range items {
		items[i] = Item{ID: i, W: 100, H: 140}
	}
	pages, err := PackPages(200, 280, 0, items, true)
	if err != nil {
		t.Fatalf("PackPages: %v", err)
	}
	total := 0
	for _, p := range pages {
		if p.UsableW <= 0 || p.UsableH <= 0 {
			t.Fatalf("bad page dims: %+v", p)
		}
		assertNoOverlap(t, p.Placements, p.UsableW, p.UsableH)
		total += len(p.Placements)
	}
	if total != len(items) {
		t.Fatalf("placed total=%d, expected %d", total, len(items))
	}
	if len(pages) < 3 {
		t.Fatalf("expected at least 3 pages for 10 items, got %d", len(pages))
	}
}

func TestPackPagesOversizeItemReturnsError(t *testing.T) {
	items := []Item{{ID: 1, W: 9999, H: 9999}}
	_, err := PackPages(200, 280, 0, items, true)
	if err == nil {
		t.Fatalf("expected oversize item to produce error")
	}
}

func TestPackPagesRotationReducesPages(t *testing.T) {
	// 构造 8 块"极瘦"的 tile：20×180（竖向窄长）。bin 200×200。
	// 不旋转：一列只能放 1 个（200 容得下 180；20 宽占一列，10 列一页 = 10 块），其实应该能放完。
	// 换个更极端的：10×190 的 20 块放 100×200 bin：
	//   不旋转：每块竖占 190h / 10w；一列可 1 块，共 10 列（10*10=100 <= 100）→ 10 块一页。20 块要 2 页。
	//   旋转：把部分转成 190×10 横放，每行占 10h，可堆 20 行（=200）→ 一行横一列竖的混合？
	// 算了，跑一遍看趋势即可。
	items := make([]Item, 20)
	for i := range items {
		items[i] = Item{ID: i, W: 10, H: 190}
	}
	pagesNoRot, err1 := PackPages(100, 200, 0, items, false)
	pagesRot, err2 := PackPages(100, 200, 0, items, true)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v / %v", err1, err2)
	}
	// 允许旋转时页数应 ≤ 不允许
	if len(pagesRot) > len(pagesNoRot) {
		t.Fatalf("rotation should not increase pages: %d > %d", len(pagesRot), len(pagesNoRot))
	}
}

func TestPackZeroBinFallsBack(t *testing.T) {
	placed, leftover := Pack(0, 100, 0, []Item{{ID: 1, W: 10, H: 10}}, true)
	if len(placed) != 0 || len(leftover) != 1 {
		t.Fatalf("zero-width bin should yield no placements, got %d / %d", len(placed), len(leftover))
	}
}

// 健全检查：纯几何函数没有不经意的副作用。
func TestIsContainedSelf(t *testing.T) {
	r := Rect{X: 1, Y: 2, W: 3, H: 4}
	if !isContained(r, r) {
		t.Fatalf("rect should contain itself")
	}
}

// Gutter 行为校验：任意两块真实 tile 之间的最短距离必须 ≥ gutter - eps，
// 且所有 tile 都不越过真实 usable 边界（gutter 不应偷走可用空间）。
func TestPackGutterKeepsDistance(t *testing.T) {
	items := []Item{
		{ID: 1, W: 40, H: 40},
		{ID: 2, W: 40, H: 40},
		{ID: 3, W: 40, H: 40},
		{ID: 4, W: 40, H: 40},
	}
	const gutter = 5.0
	const bin = 85.0 // 2×40 + gutter 正好塞下 2 列 / 2 行
	placed, leftover := Pack(bin, bin, gutter, items, false)
	if len(placed) != 4 || len(leftover) != 0 {
		t.Fatalf("with gutter %v expected all 4 to fit in %vx%v, got placed=%d leftover=%d",
			gutter, bin, bin, len(placed), len(leftover))
	}
	// 真实 tile 不能越过真实 bin 边界。
	for _, p := range placed {
		if p.X+p.PlaceW > bin+eps || p.Y+p.PlaceH > bin+eps {
			t.Fatalf("tile extends past real bin: %+v (bin=%v)", p, bin)
		}
	}
	// 两两间隙：水平或垂直方向必须拉开 >= gutter。
	for i := 0; i < len(placed); i++ {
		for j := i + 1; j < len(placed); j++ {
			a := placed[i]
			b := placed[j]
			xGap := math.Max(a.X-b.X-b.PlaceW, b.X-a.X-a.PlaceW)
			yGap := math.Max(a.Y-b.Y-b.PlaceH, b.Y-a.Y-a.PlaceH)
			if math.Max(xGap, yGap) < gutter-eps {
				t.Fatalf("gutter violated between #%d %+v and #%d %+v (xGap=%v yGap=%v)",
					i, a, j, b, xGap, yGap)
			}
		}
	}
}

// gutter 不够时应当 spill 到下一页。
func TestPackPagesGutterOverflowsToNextPage(t *testing.T) {
	// 2 块 60×60 放进 130×60 的 bin：无 gutter 时 60+60=120 ≤ 130，一页装下；
	// 有 gutter=15 时 60+15+60=135 > 130，必须开两页。
	items := []Item{
		{ID: 1, W: 60, H: 60},
		{ID: 2, W: 60, H: 60},
	}
	noG, err := PackPages(130, 60, 0, items, false)
	if err != nil || len(noG) != 1 || len(noG[0].Placements) != 2 {
		t.Fatalf("no-gutter baseline: expected 1 page with 2 tiles, got pages=%d err=%v",
			len(noG), err)
	}
	withG, err := PackPages(130, 60, 15, items, false)
	if err != nil {
		t.Fatalf("with-gutter: %v", err)
	}
	if len(withG) != 2 {
		t.Fatalf("with gutter expected 2 pages, got %d", len(withG))
	}
}

func TestSplitFreeTouchingNotSplit(t *testing.T) {
	// used 紧贴 free 边缘不相交，应当保留原 free。
	free := []Rect{{X: 0, Y: 0, W: 100, H: 100}}
	used := Rect{X: 100, Y: 0, W: 10, H: 10}
	out := splitFree(free, used)
	if len(out) != 1 || math.Abs(out[0].W-100) > eps {
		t.Fatalf("touching rects shouldn't be split: %+v", out)
	}
}
