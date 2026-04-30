// Package export 把"原图 + 裁剪 + 目标尺寸 + 纸张参数"的输入一气呵成地走完
// "裁剪 → LANCZOS 缩放 → 切块 → 写 PDF" 的管线，输出到指定路径。
//
// 本包是 app.go 与底层 imaging / tiler / packer / pdf 之间的胶水；
// 不涉及 Wails 运行时，也不做 UI 交互，从而可独立跑单测。
//
// 标准模式（ModeStandard）：一页一块，纸张方向由 Request.Landscape 决定。
// 省纸模式（ModeSaving）：MaxRects-BSSF 2D 装箱 + 90° 旋转 + 每页自动选方向，
// 目标是用最少的纸张印出所有（未被 Skip 的）图块。
package export

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"time"

	"go-printtile-pro/internal/imaging"
	"go-printtile-pro/internal/packer"
	"go-printtile-pro/internal/pdf"
	"go-printtile-pro/internal/tiler"
	"go-printtile-pro/internal/units"
)

// Rect 原图坐标下的像素矩形（半开区间 [X0, X1) × [Y0, Y1)）。
type Rect struct {
	X0 int
	Y0 int
	X1 int
	Y1 int
}

// Mode 导出模式枚举。零值 == ModeStandard，方便调用方不显式设置。
type Mode string

const (
	ModeStandard Mode = ""       // 标准模式（默认）：一页一块
	ModeSaving   Mode = "saving" // 省纸模式：2D 装箱
)

// SavingTileGapMm 省纸模式下同一张纸里相邻 tile 之间的强制间距（毫米）。
// 不和 MarginMm（页面外边距）或 OverlapMm（标准模式相邻图块重叠）混淆：
//   - MarginMm 是"纸张四周到可用区"的距离，页面级；
//   - OverlapMm 标准模式用来做拼接缓冲；
//   - SavingTileGapMm 是省纸模式里打包后 tile 之间的间距，方便裁切 / 张贴时留白。
// 3mm 是手工打印试验下来"裁切刀切偏也不会切到图"的比较舒服的值。
const SavingTileGapMm = 3.0

// Request 一次 PDF 导出的完整输入。物理长度字段命名上明示单位，避免 mm / cm 混淆。
//
// PaperWMm × PaperHMm 必须是 portrait 规范尺寸（如 A4 = 210 × 297）；
// Landscape 只对标准模式生效，决定每页方向一起翻转；
// 省纸模式下方向由 packer 每页独立决定。
type Request struct {
	Source      *imaging.LoadedImage
	Crop        *Rect
	TargetWMm   float64
	TargetHMm   float64
	PaperWMm    float64 // portrait 规范宽
	PaperHMm    float64 // portrait 规范高
	Landscape   bool    // 标准模式纸张方向；省纸模式忽略
	MarginMm    float64
	OverlapMm   float64
	Mode        Mode
	AllowRotate bool // 省纸模式是否允许 90° 旋转 tile
	Output      string
	Progress    ProgressFn
	Skip        func(col, row int) bool // 未勾选的 tile 在两种模式下都不参与输出

	// Ctx 贯穿导出；为 nil 时使用 context.Background()。
	Ctx context.Context

	// UpscaleEnabled 为 true 且 Upscale 非 nil 时，在裁剪后、目标缩放前调用 Upscale。
	UpscaleEnabled bool
	Upscale        UpscaleFn
}

// Stage 导出过程的阶段枚举，主要给进度条用。
type Stage string

const (
	StageCropping     Stage = "cropping"
	StageUploading    Stage = "uploading"
	StageUpscaling    Stage = "upscaling"
	StageDownloading  Stage = "downloading"
	StageScaling      Stage = "scaling"
	StageTiling       Stage = "tiling"
	StageEncoding     Stage = "encoding"
	StageWriting      Stage = "writing"
	StageCompleted    Stage = "completed"
)

// UpscaleFn 在裁剪完成后调用，将位图交给外部放大管线；返回放大后的 image.Image。
type UpscaleFn func(ctx context.Context, src image.Image, progress ProgressFn) (image.Image, error)

// ProgressFn 进度回调。stage 为当前阶段，percent 为 0~100 的整数。
// 回调实现需避免阻塞（上游通常在 goroutine 内同步调用）。
type ProgressFn func(stage Stage, percent int)

// Result PDF 生成的结果摘要。
type Result struct {
	OutputPath string
	Pages      int
}

// Run 执行完整导出流程。错误在任何阶段抛出都会直接返回，不做部分落盘。
func Run(req Request) (*Result, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	progress := req.Progress
	if progress == nil {
		progress = func(Stage, int) {}
	}

	dpi := req.Source.DPIX

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. 裁剪
	progress(StageCropping, 0)
	srcImg := req.Source.Image
	if req.Crop != nil {
		srcImg = imaging.Crop(srcImg, req.Crop.X0, req.Crop.Y0, req.Crop.X1, req.Crop.Y1)
	}
	progress(StageCropping, 100)

	// 1b. 可选：BigJPG 放大（在目标缩放之前插入，提高有效采样分辨率）
	if req.UpscaleEnabled && req.Upscale != nil {
		var err error
		srcImg, err = req.Upscale(ctx, srcImg, progress)
		if err != nil {
			return nil, fmt.Errorf("export: upscale: %w", err)
		}
	}

	// 2. 缩放到目标像素
	progress(StageScaling, 0)
	targetPxW := units.MmToPx(req.TargetWMm, dpi)
	targetPxH := units.MmToPx(req.TargetHMm, dpi)
	if targetPxW <= 0 || targetPxH <= 0 {
		return nil, fmt.Errorf("export: target pixel size must be positive (got %dx%d)", targetPxW, targetPxH)
	}
	scaled := imaging.ScaleTo(srcImg, targetPxW, targetPxH)
	progress(StageScaling, 100)

	// 3. 计算切片方案
	//    标准模式：按 Landscape 决定切块用的纸张方向；
	//    省纸模式：始终以 portrait 切块，让 packer 决定每页方向 + 旋转。
	progress(StageTiling, 0)
	tilePaperW, tilePaperH := req.PaperWMm, req.PaperHMm
	if req.Mode == ModeStandard && req.Landscape {
		tilePaperW, tilePaperH = tilePaperH, tilePaperW
	}
	plan, err := tiler.BuildPlan(tiler.Params{
		SourceW:   targetPxW,
		SourceH:   targetPxH,
		PaperWMm:  tilePaperW,
		PaperHMm:  tilePaperH,
		MarginMm:  req.MarginMm,
		OverlapMm: req.OverlapMm,
		DPI:       dpi,
	})
	if err != nil {
		return nil, fmt.Errorf("export: build plan: %w", err)
	}
	if len(plan.Tiles) == 0 {
		return nil, fmt.Errorf("export: no tiles to render")
	}

	// 应用 Skip 过滤（两种模式通用）：skip 的 tile 完全不进入下一步。
	kept := plan.Tiles
	if req.Skip != nil {
		kept = make([]tiler.Tile, 0, len(plan.Tiles))
		for _, t := range plan.Tiles {
			if !req.Skip(t.Col, t.Row) {
				kept = append(kept, t)
			}
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("export: all tiles are skipped, nothing to render")
	}
	progress(StageTiling, 100)

	// 4. 构建 PDF
	progress(StageEncoding, 0)
	builder, err := pdf.New(pdf.Config{
		PaperWMm:  req.PaperWMm,
		PaperHMm:  req.PaperHMm,
		MarginMm:  req.MarginMm,
		OverlapMm: req.OverlapMm,
		Landscape: req.Mode == ModeStandard && req.Landscape,
	})
	if err != nil {
		return nil, err
	}

	var pages int
	switch req.Mode {
	case ModeSaving:
		pages, err = runSaving(builder, scaled, kept, req, dpi, progress)
	default:
		pages, err = runStandard(builder, scaled, kept, dpi, progress)
	}
	if err != nil {
		return nil, err
	}

	// 5. 落盘
	progress(StageWriting, 0)
	if err := builder.Save(req.Output); err != nil {
		return nil, err
	}
	progress(StageWriting, 100)
	progress(StageCompleted, 100)

	return &Result{OutputPath: req.Output, Pages: pages}, nil
}

// runStandard 标准模式：一块一页，按 plan 的行序写入。返回页数。
func runStandard(builder *pdf.Builder, scaled image.Image, kept []tiler.Tile, dpi int, progress ProgressFn) (int, error) {
	total := len(kept)
	for i, t := range kept {
		tileImg := imaging.Crop(scaled, t.X0, t.Y0, t.X1, t.Y1)
		widthMm := units.PxToMm(t.Width(), dpi)
		heightMm := units.PxToMm(t.Height(), dpi)
		if err := builder.AddTilePage(pdf.Tile{
			Col:            t.Col,
			Row:            t.Row,
			Image:          tileImg,
			WidthMm:        widthMm,
			HeightMm:       heightMm,
			HasLeftOverlap: t.Col > 0,
			HasTopOverlap:  t.Row > 0,
		}); err != nil {
			return 0, fmt.Errorf("export: add tile R%d-C%d: %w", t.Row+1, t.Col+1, err)
		}
		progress(StageEncoding, int(float64(i+1)/float64(total)*100))
	}
	return total, nil
}

// runSaving 省纸模式：把所有 kept tile 的 mm 尺寸送入 packer，得到分页装箱结果；
// 再为每页裁出位图并调用 pdf.AddPackedPage。返回最终页数。
//
// 注意：物理尺寸用 mm 精度入 packer，保证装箱决策不被 DPI 舍入污染；
// 图像仍然用像素裁切，与最终 PDF 贴图分辨率 1:1 匹配。
func runSaving(builder *pdf.Builder, scaled image.Image, kept []tiler.Tile, req Request, dpi int, progress ProgressFn) (int, error) {
	usableW := req.PaperWMm - 2*req.MarginMm
	usableH := req.PaperHMm - 2*req.MarginMm
	if usableW <= 0 || usableH <= 0 {
		return 0, fmt.Errorf("export: margin too large for paper %vx%v (usable=%vx%v)",
			req.PaperWMm, req.PaperHMm, usableW, usableH)
	}

	// packer 的 ID 即 kept 的下标；下游可据此反查 tile 的像素区间。
	//
	// mm↔px 的整数化会产生 O(1/DPI) 的微小误差：interior tile 严格等于
	// usable 区尺寸，但 PxToMm(MmToPx(287, 300), 300) ≈ 287.02，会被 packer
	// 当作"超页面"拒收。tiler 已经保证 tile ≤ usable，这里直接 clamp 吸收误差。
	items := make([]packer.Item, len(kept))
	for i, t := range kept {
		wMm := units.PxToMm(t.Width(), dpi)
		hMm := units.PxToMm(t.Height(), dpi)
		if wMm > usableW {
			wMm = usableW
		}
		if hMm > usableH {
			hMm = usableH
		}
		items[i] = packer.Item{ID: i, W: wMm, H: hMm}
	}
	pages, err := packer.PackPages(usableW, usableH, SavingTileGapMm, items, req.AllowRotate)
	if err != nil {
		return 0, fmt.Errorf("export: pack pages: %w", err)
	}

	totalTiles := len(kept)
	writtenTiles := 0
	for _, pg := range pages {
		packedTiles := make([]pdf.PackedTile, 0, len(pg.Placements))
		for _, pl := range pg.Placements {
			t := kept[pl.Item.ID]
			tileImg := imaging.Crop(scaled, t.X0, t.Y0, t.X1, t.Y1)
			packedTiles = append(packedTiles, pdf.PackedTile{
				Col:      t.Col,
				Row:      t.Row,
				Image:    tileImg,
				WidthMm:  pl.Item.W,
				HeightMm: pl.Item.H,
				XMm:      pl.X,
				YMm:      pl.Y,
				PlaceWMm: pl.PlaceW,
				PlaceHMm: pl.PlaceH,
				Rotated:  pl.Rotated,
			})
			writtenTiles++
		}
		if err := builder.AddPackedPage(pdf.PackedPage{
			Landscape: pg.Landscape,
			UsableW:   pg.UsableW,
			UsableH:   pg.UsableH,
			Tiles:     packedTiles,
		}); err != nil {
			return 0, fmt.Errorf("export: add packed page: %w", err)
		}
		progress(StageEncoding, int(float64(writtenTiles)/float64(totalTiles)*100))
	}
	return len(pages), nil
}

// DefaultFileName 根据原图路径与时间生成默认导出文件名 "{stem}_{YYYYMMDD}.pdf"。
// 入参 now 便于单测注入。
func DefaultFileName(srcPath string, now time.Time) string {
	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "image"
	}
	return fmt.Sprintf("%s_%s.pdf", stem, now.Format("20060102"))
}

func validate(req Request) error {
	if req.Source == nil {
		return fmt.Errorf("export: source image is nil")
	}
	if req.TargetWMm <= 0 || req.TargetHMm <= 0 {
		return fmt.Errorf("export: target size must be positive")
	}
	if req.PaperWMm <= 0 || req.PaperHMm <= 0 {
		return fmt.Errorf("export: paper size must be positive")
	}
	if req.Output == "" {
		return fmt.Errorf("export: output path is empty")
	}
	if req.Crop != nil {
		if req.Crop.X1 <= req.Crop.X0 || req.Crop.Y1 <= req.Crop.Y0 {
			return fmt.Errorf("export: invalid crop rect")
		}
	}
	return nil
}
