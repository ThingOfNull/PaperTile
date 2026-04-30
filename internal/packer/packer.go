// Package packer 实现"省纸模式"需要的 2D 矩形装箱。
//
// 算法选择：MaxRects-BSSF（Best Short Side Fit）。在 Jylänki 的经典对比里（Survey of 2D
// Rectangle Packing, 2010），MaxRects-BSSF 在"尽量少页 + 合理速度"的目标函数下综合表现最好，
// 实现也比 Skyline + shelf 混合方案清晰。由于本项目单页 tile 数量通常 ≤ 30，即使 O(n² · k)
// 的剪枝 (n=free rect 数，k=item 数) 也远低于 100 ms，无需额外优化。
//
// 坐标系：原点左上，X 向右、Y 向下，单位由调用方决定（本项目传入毫米）。
// 所有数值比较都走 eps 容差，避免浮点累加后误判为"装不下"。
package packer

import (
	"fmt"
	"math"
	"sort"
)

// eps 浮点比较容差，单位与输入一致（毫米）。0.0001mm 远低于实际打印精度，足够吸收累加误差。
const eps = 1e-4

// Rect 自由矩形 / 占用矩形的统一表示。
type Rect struct {
	X, Y, W, H float64
}

// Item 待装箱的条目。ID 由调用方维护，装箱后可据此回溯原始 tile。
type Item struct {
	ID int
	W  float64
	H  float64
}

// Placement 一个 item 在某一 bin 中的最终位置。PlaceW/H 已考虑旋转，可直接用来绘制。
type Placement struct {
	Item    Item
	X, Y    float64
	PlaceW  float64
	PlaceH  float64
	Rotated bool // true = 输入的 W,H 交换过；绘制时需把图像 90° 顺时针旋转
}

// Page 表示一张纸对应的装箱结果。
type Page struct {
	UsableW   float64 // 可用区宽（= 页宽 − 2×边距，已根据 Landscape 交换）
	UsableH   float64 // 可用区高
	Landscape bool
	Placements []Placement
}

// Pack 把尽可能多的 items 装入 (binW × binH) 的一个 bin。
// 返回已放置的 Placement 列表与"放不下"的 leftover（保留原始输入顺序）。
// allowRotate 为 true 时每个 item 允许 90° 放置。
// gutter 为 tile 之间强制留出的间隙（单位与 bin 一致），0 = 紧贴。
//
// "充气装箱"实现：内部把每个 item 扩成 (W+g) × (H+g)，bin 扩成 (binW+g) × (binH+g)，
// 再跑标准 MaxRects。这样相邻 tile 的扩展 bbox 紧贴就等同于真实 tile 相距 g；
// 最边上的 tile 真实坐标刚好到达 usable 边界，不浪费任何空间。
//
// 注意：本函数会对 items 做"最长边降序"预排序以提升装箱率，
// 但 leftover 仍然按"原始输入里没被选中的 item"顺序返回，方便调用方复用。
func Pack(binW, binH, gutter float64, items []Item, allowRotate bool) ([]Placement, []Item) {
	if binW <= eps || binH <= eps {
		return nil, append([]Item(nil), items...)
	}
	if gutter < 0 {
		gutter = 0
	}
	g := gutter

	// 建索引：内部排序后处理，结束后用索引恢复"原始顺序里谁没被放"。
	type indexed struct {
		idx  int
		item Item
	}
	sorted := make([]indexed, len(items))
	for i, it := range items {
		sorted[i] = indexed{idx: i, item: it}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		mi := math.Max(sorted[i].item.W, sorted[i].item.H)
		mj := math.Max(sorted[j].item.W, sorted[j].item.H)
		if math.Abs(mi-mj) > eps {
			return mi > mj
		}
		return sorted[i].item.W*sorted[i].item.H > sorted[j].item.W*sorted[j].item.H
	})

	// 充气 bin：在右/下方多出 g，这样最边缘 tile 真实右/下恰好 = 原 binW/binH。
	free := []Rect{{X: 0, Y: 0, W: binW + g, H: binH + g}}
	var placed []Placement
	placedIdx := make(map[int]struct{}, len(items))

	for _, s := range sorted {
		it := s.item
		bestIdx := -1
		bestScore1 := math.MaxFloat64
		bestScore2 := math.MaxFloat64
		var bestX, bestY, bestW, bestH float64
		var bestRotated bool

		tryFit := func(w, h float64, rotated bool) {
			for idx, f := range free {
				if w > f.W+eps || h > f.H+eps {
					continue
				}
				// BSSF：占用后"短边剩余"最小；长边剩余作 tie break。
				remW := f.W - w
				remH := f.H - h
				s1 := math.Min(remW, remH)
				s2 := math.Max(remW, remH)
				if s1 < bestScore1-eps ||
					(math.Abs(s1-bestScore1) <= eps && s2 < bestScore2-eps) {
					bestScore1 = s1
					bestScore2 = s2
					bestIdx = idx
					bestX = f.X
					bestY = f.Y
					bestW = w
					bestH = h
					bestRotated = rotated
				}
			}
		}

		// 内部用"充气尺寸"参与装箱；放置坐标保持；Placement.PlaceW/H 还原为真实尺寸。
		tryFit(it.W+g, it.H+g, false)
		if allowRotate && math.Abs(it.W-it.H) > eps {
			tryFit(it.H+g, it.W+g, true)
		}
		if bestIdx < 0 {
			continue
		}

		placed = append(placed, Placement{
			Item:    it,
			X:       bestX,
			Y:       bestY,
			PlaceW:  bestW - g,
			PlaceH:  bestH - g,
			Rotated: bestRotated,
		})
		placedIdx[s.idx] = struct{}{}

		// 注意：切 free 时用"充气 used"，保证后续 item 之间自动留出 g。
		used := Rect{X: bestX, Y: bestY, W: bestW, H: bestH}
		free = splitFree(free, used)
		free = pruneFree(free)
	}

	// 按"原始输入顺序"收集 leftover，方便调用方继续送到下一页。
	leftover := make([]Item, 0, len(items)-len(placed))
	for i, it := range items {
		if _, ok := placedIdx[i]; !ok {
			leftover = append(leftover, it)
		}
	}
	return placed, leftover
}

// PackPages 把 items 逐页装入纸张，直到全部放下。
// usableW / usableH 是 portrait 方向下的可用区域（= 纸宽 − 2m × 纸高 − 2m）。
// gutter 为同一页上 tile 之间的最小间距（mm），方便裁切 / 张贴时留出边距。
// 每页独立选 portrait / landscape：分别跑一次 Pack，取"放得多"的方向；
// 数量相等时偏好 portrait（保持整份 PDF 视觉更一致）。
//
// 任何单个 item 超过页面最大维度（即使旋转后也装不下）→ 返回错误，避免死循环。
func PackPages(usableW, usableH, gutter float64, items []Item, allowRotate bool) ([]Page, error) {
	if usableW <= eps || usableH <= eps {
		return nil, fmt.Errorf("packer: usable area must be positive (got %vx%v)", usableW, usableH)
	}
	remaining := append([]Item(nil), items...)
	var pages []Page
	for len(remaining) > 0 {
		// portrait
		portPlaced, portLeft := Pack(usableW, usableH, gutter, remaining, allowRotate)
		// landscape：宽高交换
		landPlaced, landLeft := Pack(usableH, usableW, gutter, remaining, allowRotate)

		var chosenPlaced []Placement
		var chosenLeft []Item
		landscape := false
		if len(portPlaced) >= len(landPlaced) {
			chosenPlaced, chosenLeft = portPlaced, portLeft
		} else {
			chosenPlaced, chosenLeft = landPlaced, landLeft
			landscape = true
		}

		if len(chosenPlaced) == 0 {
			// 两种方向一个都装不下 → 说明首个 item 超过了纸张尺寸，上游应当保证不会走到这里。
			oversize := remaining[0]
			return nil, fmt.Errorf("packer: item id=%d size %vx%v exceeds page usable area %vx%v",
				oversize.ID, oversize.W, oversize.H, usableW, usableH)
		}

		pageW, pageH := usableW, usableH
		if landscape {
			pageW, pageH = usableH, usableW
		}
		pages = append(pages, Page{
			UsableW:    pageW,
			UsableH:    pageH,
			Landscape:  landscape,
			Placements: chosenPlaced,
		})
		remaining = chosenLeft
	}
	return pages, nil
}

// splitFree 对每个与 used 相交的自由矩形切成最多 4 块（上/下/左/右）。
// 注意：标准 MaxRects 会切 4 块，个别会被 pruneFree 合并或淘汰。
func splitFree(free []Rect, used Rect) []Rect {
	out := make([]Rect, 0, len(free)*2)
	for _, f := range free {
		if !intersects(f, used) {
			out = append(out, f)
			continue
		}
		// 上
		if used.Y > f.Y+eps {
			out = append(out, Rect{X: f.X, Y: f.Y, W: f.W, H: used.Y - f.Y})
		}
		// 下
		if used.Y+used.H < f.Y+f.H-eps {
			out = append(out, Rect{
				X: f.X,
				Y: used.Y + used.H,
				W: f.W,
				H: f.Y + f.H - (used.Y + used.H),
			})
		}
		// 左
		if used.X > f.X+eps {
			out = append(out, Rect{X: f.X, Y: f.Y, W: used.X - f.X, H: f.H})
		}
		// 右
		if used.X+used.W < f.X+f.W-eps {
			out = append(out, Rect{
				X: used.X + used.W,
				Y: f.Y,
				W: f.X + f.W - (used.X + used.W),
				H: f.H,
			})
		}
	}
	return out
}

func intersects(a, b Rect) bool {
	return a.X < b.X+b.W-eps && b.X < a.X+a.W-eps &&
		a.Y < b.Y+b.H-eps && b.Y < a.Y+a.H-eps
}

// pruneFree 去掉退化（0 尺寸）和被其他自由矩形完全包含的矩形。
// O(n²) 实现；实际 n 很少超过 20，完全可接受。
func pruneFree(free []Rect) []Rect {
	nonDeg := make([]Rect, 0, len(free))
	for _, r := range free {
		if r.W > eps && r.H > eps {
			nonDeg = append(nonDeg, r)
		}
	}
	out := make([]Rect, 0, len(nonDeg))
	for i, a := range nonDeg {
		contained := false
		for j, b := range nonDeg {
			if i == j {
				continue
			}
			if isContained(a, b) {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, a)
		}
	}
	return out
}

func isContained(a, b Rect) bool {
	return a.X >= b.X-eps && a.Y >= b.Y-eps &&
		a.X+a.W <= b.X+b.W+eps && a.Y+a.H <= b.Y+b.H+eps
}
