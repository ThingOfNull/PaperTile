// Package tiler 负责将"缩放后的图像"按"纸张、边距、重叠量"切成若干图块，
// 并以二维网格形式返回每块的像素矩形。本包只做纯数学运算，不持有图像数据。
//
// 坐标约定：
//   - 像素坐标原点在图像左上角，X 向右、Y 向下，半开区间 [X0, X1) × [Y0, Y1)。
//   - 网格坐标 (Col, Row) 从 0 开始。
//
// 重叠约束（PRD v2 §5.1）：
//   - 有效打印区 We = PaperW − 2M；He = PaperH − 2M（均为毫米）。
//   - 步进 Ws = We − Overlap；Hs = He − Overlap。
//   - 第 i 列起点像素 = i × Ws × DPI / 25.4。
//   - 列数 = max(1, ⌈(SrcW − TileW) / StepW⌉ + 1)，超出部分 clamp 到源图边界。
package tiler

import (
	"errors"
	"math"

	"go-printtile-pro/internal/units"
)

// Tile 表示单个图块在缩放后源图中的像素矩形及其网格坐标。
type Tile struct {
	Col int // 从 0 开始的列号
	Row int // 从 0 开始的行号
	X0  int // 包含
	Y0  int // 包含
	X1  int // 不包含
	Y1  int // 不包含
}

// Width 返回图块像素宽。
func (t Tile) Width() int { return t.X1 - t.X0 }

// Height 返回图块像素高。
func (t Tile) Height() int { return t.Y1 - t.Y0 }

// Params 描述一次切片请求的输入参数。
// 所有物理长度单位为毫米；SourceW/H 为缩放后的源图像素尺寸。
type Params struct {
	SourceW   int     // 源图像素宽（已按目标物理尺寸缩放后）
	SourceH   int     // 源图像素高
	PaperWMm  float64 // 纸张宽（毫米）
	PaperHMm  float64 // 纸张高（毫米）
	MarginMm  float64 // 四周打印边距（毫米）
	OverlapMm float64 // 相邻图块重叠量（毫米，水平 = 垂直）
	DPI       int     // 生效 DPI，用于 mm → px 换算
}

// Plan 描述一次切片方案的计算结果。
type Plan struct {
	Params     Params // 回传计算使用的参数（便于前端同步显示）
	Cols       int    // 列数
	Rows       int    // 行数
	TilePxW    int    // 单块像素宽（完整块；最后一列可能被 clamp）
	TilePxH    int    // 单块像素高
	StepPxX    int    // 水平步进像素
	StepPxY    int    // 垂直步进像素
	OverlapPxX int    // 水平重叠像素
	OverlapPxY int    // 垂直重叠像素
	Tiles      []Tile // 从左上到右下按行优先展开的所有图块
}

// ErrInvalidSize 源图或纸张尺寸无效。
var ErrInvalidSize = errors.New("tiler: source or paper size must be positive")

// ErrMarginTooLarge 边距过大导致没有可打印区。
var ErrMarginTooLarge = errors.New("tiler: margin too large, no printable area")

// ErrOverlapOutOfRange 重叠量超出有效打印区。
var ErrOverlapOutOfRange = errors.New("tiler: overlap must be in [0, effective print size)")

// ErrInvalidDPI DPI 非正数。
var ErrInvalidDPI = errors.New("tiler: dpi must be positive")

// BuildPlan 根据参数计算切片方案。
// 若参数非法返回错误；否则返回按行优先排列的图块集合。
func BuildPlan(p Params) (*Plan, error) {
	if p.SourceW <= 0 || p.SourceH <= 0 || p.PaperWMm <= 0 || p.PaperHMm <= 0 {
		return nil, ErrInvalidSize
	}
	if p.DPI <= 0 {
		return nil, ErrInvalidDPI
	}
	effWMm := p.PaperWMm - 2*p.MarginMm
	effHMm := p.PaperHMm - 2*p.MarginMm
	if effWMm <= 0 || effHMm <= 0 {
		return nil, ErrMarginTooLarge
	}
	if p.OverlapMm < 0 || p.OverlapMm >= effWMm || p.OverlapMm >= effHMm {
		return nil, ErrOverlapOutOfRange
	}

	tileW := units.MmToPx(effWMm, p.DPI)
	tileH := units.MmToPx(effHMm, p.DPI)
	stepX := units.MmToPx(effWMm-p.OverlapMm, p.DPI)
	stepY := units.MmToPx(effHMm-p.OverlapMm, p.DPI)
	overlapX := units.MmToPx(p.OverlapMm, p.DPI)
	overlapY := units.MmToPx(p.OverlapMm, p.DPI)

	// 理论上经过上面的校验 step > 0，这里做一次防御性检查，避免除零。
	if stepX <= 0 || stepY <= 0 || tileW <= 0 || tileH <= 0 {
		return nil, ErrOverlapOutOfRange
	}

	cols := computeCount(p.SourceW, tileW, stepX)
	rows := computeCount(p.SourceH, tileH, stepY)

	tiles := make([]Tile, 0, cols*rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0 := c * stepX
			y0 := r * stepY
			x1 := x0 + tileW
			y1 := y0 + tileH
			if x1 > p.SourceW {
				x1 = p.SourceW
			}
			if y1 > p.SourceH {
				y1 = p.SourceH
			}
			if x0 >= x1 || y0 >= y1 {
				// 正常不会触发，留作防御：若步进累计超过源图则跳过。
				continue
			}
			tiles = append(tiles, Tile{
				Col: c, Row: r,
				X0: x0, Y0: y0,
				X1: x1, Y1: y1,
			})
		}
	}

	return &Plan{
		Params:     p,
		Cols:       cols,
		Rows:       rows,
		TilePxW:    tileW,
		TilePxH:    tileH,
		StepPxX:    stepX,
		StepPxY:    stepY,
		OverlapPxX: overlapX,
		OverlapPxY: overlapY,
		Tiles:      tiles,
	}, nil
}

// computeCount 返回覆盖 total 像素所需的块数。
// 单块长度为 tile，步进为 step；当 total ≤ tile 时只需 1 块。
//
//	块 i 的覆盖区间为 [i*step, i*step + tile)，需满足最后一块覆盖到 total。
//	即 (cols-1)*step + tile ≥ total，故 cols ≥ ⌈(total - tile) / step⌉ + 1。
func computeCount(total, tile, step int) int {
	if total <= tile {
		return 1
	}
	remaining := total - tile
	return int(math.Ceil(float64(remaining)/float64(step))) + 1
}
