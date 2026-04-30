package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"log"
	"path/filepath"
	"sync"
	"time"

	"go-printtile-pro/internal/export"
	"go-printtile-pro/internal/imaging"
	"go-printtile-pro/internal/packer"
	"go-printtile-pro/internal/secrets"
	"go-printtile-pro/internal/tiler"
	"go-printtile-pro/internal/units"
	"go-printtile-pro/internal/upscale"

	xi "github.com/disintegration/imaging"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// previewLongSide 前端预览图长边像素上限。超过该值会按比例缩小，避免 base64 payload 过大。
const previewLongSide = 1400

// App Wails 应用的根结构，持有运行时上下文与"当前图片"状态。
// 本版本采用最简单的单例模型：同一时刻只能打开一张图片。
type App struct {
	ctx context.Context

	mu          sync.RWMutex
	current     *imaging.LoadedImage
	currentPath string // 当前已加载图片的原始路径，用于导出时推导默认文件名

	secretsCfg *secrets.Config // 本地配置文件中的第三方配置（百度智能云等）
}

// NewApp 创建 App 实例。Wails main.go 会调用它并注册生命周期回调。
func NewApp() *App {
	return &App{}
}

// startup 在 Wails 启动时被调用，保存上下文以便后续使用 runtime API。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cfg, err := secrets.Load()
	if err == nil {
		a.secretsCfg = cfg
	}
}

// BaiduConfigPayload 前后端交换的百度 AK/SK 配置。
type BaiduConfigPayload struct {
	APIKey     string `json:"apiKey"`
	SecretKey  string `json:"secretKey"`
	Configured bool   `json:"configured"`
	ConfigPath string `json:"configPath"`
}

// GetBaiduConfig 返回当前内存中的百度配置（用于前端回填弹窗）。
func (a *App) GetBaiduConfig() BaiduConfigPayload {
	if a.secretsCfg == nil {
		return BaiduConfigPayload{
			Configured: false,
			ConfigPath: secrets.LocalConfigPath(),
		}
	}
	return BaiduConfigPayload{
		APIKey:     a.secretsCfg.Baidu.APIKey,
		SecretKey:  a.secretsCfg.Baidu.SecretKey,
		Configured: a.secretsCfg.UpscaleReady(),
		ConfigPath: secrets.LocalConfigPath(),
	}
}

// SaveBaiduConfig 保存百度 AK/SK 到本地配置文件。
func (a *App) SaveBaiduConfig(payload BaiduConfigPayload) (BaiduConfigPayload, error) {
	if payload.APIKey == "" || payload.SecretKey == "" {
		return BaiduConfigPayload{}, fmt.Errorf("baidu config: apiKey / secretKey 不能为空")
	}
	cfg := &secrets.Config{
		Baidu: secrets.BaiduConfig{
			APIKey:    payload.APIKey,
			SecretKey: payload.SecretKey,
		},
	}
	if err := secrets.Save(cfg); err != nil {
		return BaiduConfigPayload{}, err
	}
	a.secretsCfg = cfg
	return a.GetBaiduConfig(), nil
}

// AppFeatures 前端用于判断是否展示可选功能（如百度图像增强）。
type AppFeatures struct {
	BaiduUpscale bool   `json:"baiduUpscale"`
	UpscaleHint  string `json:"upscaleHint,omitempty"` // 未就绪时给人看的说明（已就绪为空）
}

// GetAppFeatures 返回当前运行实例已启用的能力位。
func (a *App) GetAppFeatures() AppFeatures {
	if a.secretsCfg == nil || !a.secretsCfg.UpscaleReady() {
		return AppFeatures{
			BaiduUpscale: false,
			UpscaleHint:  secrets.UpscaleNotReadyHint(a.secretsCfg),
		}
	}
	return AppFeatures{BaiduUpscale: true}
}

// ImageInfo 暴露给前端的图片元数据。
// 其中 PreviewDataURL 是限制在 previewLongSide 以内的 PNG base64（data URL 形式），
// 给主画布即时渲染用，不是原图。原图保留在后端，后续切片直接使用。
type ImageInfo struct {
	Path           string `json:"path"`
	Format         string `json:"format"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	DPIX           int    `json:"dpiX"`
	DPIY           int    `json:"dpiY"`
	RawDPIX        int    `json:"rawDpiX"`
	RawDPIY        int    `json:"rawDpiY"`
	PreviewWidth   int    `json:"previewWidth"`
	PreviewHeight  int    `json:"previewHeight"`
	PreviewDataURL string `json:"previewDataUrl"`
}

// OpenImageDialog 弹出系统文件选择对话框，用户确认后自动加载图片。
// 返回 ImageInfo 给前端直接渲染；若用户取消则返回 (nil, nil)。
func (a *App) OpenImageDialog() (*ImageInfo, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择要切分的图片",
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "图片文件 (*.jpg;*.jpeg;*.png;*.bmp;*.tif;*.tiff;*.webp)",
				Pattern:     "*.jpg;*.jpeg;*.png;*.bmp;*.tif;*.tiff;*.webp",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open dialog: %w", err)
	}
	if path == "" {
		return nil, nil
	}
	return a.LoadImage(path)
}

// LoadImage 从指定路径加载图片，提取 DPI，生成预览并缓存原图到内存。
func (a *App) LoadImage(path string) (*ImageInfo, error) {
	loaded, err := imaging.Load(path)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.current = loaded
	a.currentPath = path
	a.mu.Unlock()

	preview, pw, ph, err := buildPreview(loaded.Image)
	if err != nil {
		return nil, fmt.Errorf("encode preview: %w", err)
	}

	return &ImageInfo{
		Path:           path,
		Format:         loaded.Format,
		Width:          loaded.Width,
		Height:         loaded.Height,
		DPIX:           loaded.DPIX,
		DPIY:           loaded.DPIY,
		RawDPIX:        loaded.RawDPIX,
		RawDPIY:        loaded.RawDPIY,
		PreviewWidth:   pw,
		PreviewHeight:  ph,
		PreviewDataURL: preview,
	}, nil
}

// PlanRequest 前端请求切片方案时携带的参数。
// 字段顺序对应 PRD v2 §4 的输入流程：裁剪 → 目标物理尺寸 → 纸张 → 边距 / 重叠 → 模式参数。
//
// 注意：PaperWidthMm / PaperHeightMm 传入的是"切片使用的纸张方向"（前端 effectivePaper），
// 省纸模式下固定等于 portrait 规范尺寸；标准模式可能已按 landscape 交换。
// 这样 tiler 的几何约束不需要感知 landscape / mode，保持纯函数属性。
type PlanRequest struct {
	Crop *CropRect `json:"crop,omitempty"`

	TargetWidthCm  float64 `json:"targetWidthCm,omitempty"`
	TargetHeightCm float64 `json:"targetHeightCm,omitempty"`

	PaperWidthMm  float64 `json:"paperWidthMm"`
	PaperHeightMm float64 `json:"paperHeightMm"`

	MarginMm  float64 `json:"marginMm"`
	OverlapMm float64 `json:"overlapMm"`

	// 省纸模式预览所需；标准模式忽略。
	// Mode == "saving" 时 BuildPlan 会额外跑一次装箱并填充 PlanResponse.PackedPages。
	Mode         string      `json:"mode,omitempty"`
	AllowRotate  bool        `json:"allowRotate,omitempty"`
	SkippedTiles []TileCoord `json:"skippedTiles,omitempty"`
}

// CropRect 裁剪矩形的像素坐标（基于原图，半开区间 [X0, X1) × [Y0, Y1)）。
type CropRect struct {
	X0 int `json:"x0"`
	Y0 int `json:"y0"`
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
}

// PlanResponse BuildPlan 返回前端的切片方案。
// 坐标均为"缩放后源图"的像素坐标，便于前端按 PreviewWidth/Height 等比例映射。
//
// PackedPages 仅在 PlanRequest.Mode == "saving" 时有值，表示 2D 装箱后的实际页面布局；
// 标准模式下恒为 nil，前端据此判断用哪种预览视图。
type PlanResponse struct {
	Cols        int              `json:"cols"`
	Rows        int              `json:"rows"`
	SourceW     int              `json:"sourceW"`
	SourceH     int              `json:"sourceH"`
	TilePxW     int              `json:"tilePxW"`
	TilePxH     int              `json:"tilePxH"`
	StepPxX     int              `json:"stepPxX"`
	StepPxY     int              `json:"stepPxY"`
	OverlapPxX  int              `json:"overlapPxX"`
	OverlapPxY  int              `json:"overlapPxY"`
	Tiles       []PlanTileView   `json:"tiles"`
	PackedPages []PackedPageView `json:"packedPages,omitempty"`
	Warnings    []string         `json:"warnings"`
}

// PlanTileView 单个图块在前端预览上所需的字段。
type PlanTileView struct {
	Col int `json:"col"`
	Row int `json:"row"`
	X0  int `json:"x0"`
	Y0  int `json:"y0"`
	X1  int `json:"x1"`
	Y1  int `json:"y1"`
}

// PackedPageView 省纸模式下一张 PDF 页的布局，所有长度单位均为毫米。
//
//	UsableWMm × UsableHMm：已去掉 margin 的可用区；Landscape 为 true 时宽高互换
//	（等价 portrait 规范尺寸转 90°）。
type PackedPageView struct {
	Index     int              `json:"index"`     // 0 起始的页序
	Landscape bool             `json:"landscape"` // true = 该页使用横向纸张
	UsableWMm float64          `json:"usableWMm"`
	UsableHMm float64          `json:"usableHMm"`
	Tiles     []PackedTileView `json:"tiles"`
}

// PackedTileView 一个已放置的图块。
//
//	X0..Y1：源图（缩放后）像素区间，前端复用主预览图做 CSS 裁切式缩略图。
//	XMm / YMm：相对"可用区左上"的位置（已去 margin）。
//	PlaceWMm / PlaceHMm：放置后的宽高，Rotated=true 时 = 原始 HeightMm × WidthMm。
type PackedTileView struct {
	Col      int     `json:"col"`
	Row      int     `json:"row"`
	X0       int     `json:"x0"`
	Y0       int     `json:"y0"`
	X1       int     `json:"x1"`
	Y1       int     `json:"y1"`
	XMm      float64 `json:"xMm"`
	YMm      float64 `json:"yMm"`
	PlaceWMm float64 `json:"placeWMm"`
	PlaceHMm float64 `json:"placeHMm"`
	Rotated  bool    `json:"rotated"`
}

// BuildPlan 依据前端参数计算切片方案。不修改任何状态，可被 UI 实时调用（节流由前端处理）。
// 当前阶段只做数学计算，不生成 PDF。PDF 流程将在 Step 3 引入。
func (a *App) BuildPlan(req PlanRequest) (*PlanResponse, error) {
	a.mu.RLock()
	src := a.current
	a.mu.RUnlock()
	if src == nil {
		return nil, fmt.Errorf("no image loaded")
	}

	// 1. 有效源图尺寸（像素）：裁剪框面积或整图。
	srcW, srcH := src.Width, src.Height
	if req.Crop != nil {
		srcW = req.Crop.X1 - req.Crop.X0
		srcH = req.Crop.Y1 - req.Crop.Y0
		if srcW <= 0 || srcH <= 0 {
			return nil, fmt.Errorf("invalid crop rect")
		}
	}

	// 2. 目标物理尺寸对应的缩放后像素；若未指定则等同于源尺寸。
	//    使用 DPIX 作为 X/Y 方向的统一 DPI，因为 PRD 约束 DPI 缺失或 <300 均会被
	//    规范化到同值，极端情况下原图 DPIX ≠ DPIY 也以宽方向为准、不会影响打印结果。
	dpi := src.DPIX
	scaledW, scaledH := srcW, srcH
	warnings := make([]string, 0)
	if req.TargetWidthCm > 0 && req.TargetHeightCm > 0 {
		scaledW = units.CmToPx(req.TargetWidthCm, dpi)
		scaledH = units.CmToPx(req.TargetHeightCm, dpi)
		// 低分辨率告警：放大超过 5% 视为明显拉伸。
		if float64(scaledW) > float64(srcW)*1.05 || float64(scaledH) > float64(srcH)*1.05 {
			warnings = append(warnings, "原图像素偏低，可能无法满足目标尺寸要求，打印可能模糊")
		}
	}

	// 3. 调用 tiler 计算切片方案。
	plan, err := tiler.BuildPlan(tiler.Params{
		SourceW:   scaledW,
		SourceH:   scaledH,
		PaperWMm:  req.PaperWidthMm,
		PaperHMm:  req.PaperHeightMm,
		MarginMm:  req.MarginMm,
		OverlapMm: req.OverlapMm,
		DPI:       dpi,
	})
	if err != nil {
		return nil, err
	}

	tiles := make([]PlanTileView, 0, len(plan.Tiles))
	for _, t := range plan.Tiles {
		tiles = append(tiles, PlanTileView{
			Col: t.Col, Row: t.Row,
			X0: t.X0, Y0: t.Y0,
			X1: t.X1, Y1: t.Y1,
		})
	}

	// 4. 省纸模式下追加一遍 2D 装箱，给前端实时预览页面布局。
	//    仅做物理尺寸层面的装箱，不裁图、不编码；计算量对 O(tile²) 级别远低于 1 ms。
	var packedPages []PackedPageView
	if req.Mode == "saving" {
		packedPages, err = buildPackedPreview(plan, req, dpi)
		if err != nil {
			// 装箱失败不应阻止标准切片返回；把错误转成 warning 让用户看见原因。
			warnings = append(warnings, "省纸模式装箱失败："+err.Error())
		}
	}

	return &PlanResponse{
		Cols:        plan.Cols,
		Rows:        plan.Rows,
		SourceW:     plan.Params.SourceW,
		SourceH:     plan.Params.SourceH,
		TilePxW:     plan.TilePxW,
		TilePxH:     plan.TilePxH,
		StepPxX:     plan.StepPxX,
		StepPxY:     plan.StepPxY,
		OverlapPxX:  plan.OverlapPxX,
		OverlapPxY:  plan.OverlapPxY,
		Tiles:       tiles,
		PackedPages: packedPages,
		Warnings:    warnings,
	}, nil
}

// buildPackedPreview 复用导出管线里的打包逻辑给前端做实时预览。
//
//	输入的 plan 已包含所有 tile 的像素坐标；本函数把它们的物理尺寸（mm）送进 packer，
//	输出的 PackedPageView 保留"像素坐标 + mm 放置坐标"两套，让前端同时具备：
//	- 像素坐标 → CSS 背景裁切式缩略图
//	- mm 坐标 → 可视化页面布局
//
//	与 export.runSaving 保持同样的 clamp 处理，避免 tile 的 PxToMm 误差把 packer 搞炸。
func buildPackedPreview(plan *tiler.Plan, req PlanRequest, dpi int) ([]PackedPageView, error) {
	usableW := req.PaperWidthMm - 2*req.MarginMm
	usableH := req.PaperHeightMm - 2*req.MarginMm
	if usableW <= 0 || usableH <= 0 {
		return nil, fmt.Errorf("margin too large for paper")
	}

	// 把前端传来的 skippedTiles 转成 map；未命中则进入装箱。
	skip := make(map[[2]int]struct{}, len(req.SkippedTiles))
	for _, tc := range req.SkippedTiles {
		skip[[2]int{tc.Col, tc.Row}] = struct{}{}
	}

	// kept 记录"送进 packer 的那几块 tile"，下标即 packer.Item.ID，用于反查像素坐标。
	kept := make([]tiler.Tile, 0, len(plan.Tiles))
	items := make([]packer.Item, 0, len(plan.Tiles))
	for _, t := range plan.Tiles {
		if _, ok := skip[[2]int{t.Col, t.Row}]; ok {
			continue
		}
		wMm := units.PxToMm(t.Width(), dpi)
		hMm := units.PxToMm(t.Height(), dpi)
		if wMm > usableW {
			wMm = usableW
		}
		if hMm > usableH {
			hMm = usableH
		}
		items = append(items, packer.Item{ID: len(kept), W: wMm, H: hMm})
		kept = append(kept, t)
	}
	if len(items) == 0 {
		// 全 skip：前端的"全部勾掉"保护已经拦截过，这里兜底返回空切片，不算错误。
		return []PackedPageView{}, nil
	}

	pages, err := packer.PackPages(usableW, usableH, export.SavingTileGapMm, items, req.AllowRotate)
	if err != nil {
		return nil, err
	}

	views := make([]PackedPageView, 0, len(pages))
	for i, pg := range pages {
		tiles := make([]PackedTileView, 0, len(pg.Placements))
		for _, pl := range pg.Placements {
			t := kept[pl.Item.ID]
			tiles = append(tiles, PackedTileView{
				Col:      t.Col,
				Row:      t.Row,
				X0:       t.X0,
				Y0:       t.Y0,
				X1:       t.X1,
				Y1:       t.Y1,
				XMm:      pl.X,
				YMm:      pl.Y,
				PlaceWMm: pl.PlaceW,
				PlaceHMm: pl.PlaceH,
				Rotated:  pl.Rotated,
			})
		}
		views = append(views, PackedPageView{
			Index:     i,
			Landscape: pg.Landscape,
			UsableWMm: pg.UsableW,
			UsableHMm: pg.UsableH,
			Tiles:     tiles,
		})
	}
	return views, nil
}

// ExportRequest 前端发起 PDF 导出的参数。
//
// 注意：PaperWidthMm / PaperHeightMm 必须是 portrait 规范尺寸（A4 = 210 × 297）；
// 横向通过 Landscape 字段声明，不要前端自行交换。
// 省纸模式下 Landscape 被忽略（packer 每页独立决定方向）。
type ExportRequest struct {
	Crop           *CropRect   `json:"crop,omitempty"`
	TargetWidthCm  float64     `json:"targetWidthCm"`
	TargetHeightCm float64     `json:"targetHeightCm"`
	PaperWidthMm   float64     `json:"paperWidthMm"`  // portrait 规范宽
	PaperHeightMm  float64     `json:"paperHeightMm"` // portrait 规范高
	Landscape      bool        `json:"landscape,omitempty"`
	MarginMm       float64     `json:"marginMm"`
	OverlapMm      float64     `json:"overlapMm"`
	Mode           string      `json:"mode,omitempty"`        // "standard" | "saving"
	AllowRotate    bool        `json:"allowRotate,omitempty"` // 省纸模式是否允许 90° 旋转
	SkippedTiles   []TileCoord `json:"skippedTiles,omitempty"`
	Upscale        bool        `json:"upscale"` // 导出前经百度图像清晰度增强（须已配置 AK/SK）
}

// TileCoord 标识一个图块在切片方案中的位置（0 起始的行列索引）。
type TileCoord struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

// ExportResponse 导出结果。Cancelled 为 true 表示用户在保存对话框中主动取消，不视为错误。
type ExportResponse struct {
	OutputPath string `json:"outputPath"`
	Pages      int    `json:"pages"`
	Cancelled  bool   `json:"cancelled"`
}

// exportProgressEvent 前端监听的进度事件名。负载为 { stage, percent, detail }。
const exportProgressEvent = "export:progress"

// ExportPDF 执行完整的 PDF 导出流程。流程：
//
//  1. 校验是否已有图片；
//  2. 弹 Windows 保存对话框，让用户选择输出路径（支持默认文件名 {stem}_{YYYYMMDD}.pdf）；
//  3. 调用 export.Run，把裁剪 → 缩放 → 切块 → PDF 串起来；
//  4. 进度通过 Wails 事件 export:progress 推送给前端。
//
// 返回的 ExportResponse.Cancelled 为 true 表示用户取消保存对话框，前端据此静默处理。
func (a *App) ExportPDF(req ExportRequest) (*ExportResponse, error) {
	a.mu.RLock()
	src := a.current
	srcPath := a.currentPath
	a.mu.RUnlock()
	if src == nil {
		return nil, fmt.Errorf("no image loaded")
	}

	// 1. 弹保存对话框
	defaultName := export.DefaultFileName(srcPath, time.Now())
	defaultDir := ""
	if srcPath != "" {
		defaultDir = filepath.Dir(srcPath)
	}
	out, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "保存 PDF",
		DefaultFilename:  defaultName,
		DefaultDirectory: defaultDir,
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "PDF 文件 (*.pdf)",
				Pattern:     "*.pdf",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("save dialog: %w", err)
	}
	if out == "" {
		return &ExportResponse{Cancelled: true}, nil
	}

	// 已勾选放大但嵌入配置未就绪：必须报错，禁止静默跳过云端增强。
	if req.Upscale {
		if a.secretsCfg == nil || !a.secretsCfg.UpscaleReady() {
			return nil, fmt.Errorf(
				"已勾选「AI 清晰度增强」，但本地未配置有效的百度 AK/SK。\n请先点击「配置百度云」并保存 baidu.apiKey / baidu.secretKey。\n详情：%s",
				secrets.UpscaleNotReadyHint(a.secretsCfg),
			)
		}
	}

	// 2. 组装 export.Request。cm → mm 由 app.go 统一完成，下游只接受 mm。
	var cropRect *export.Rect
	if req.Crop != nil {
		cropRect = &export.Rect{
			X0: req.Crop.X0, Y0: req.Crop.Y0,
			X1: req.Crop.X1, Y1: req.Crop.Y1,
		}
	}

	// 3. 拣选模式：未勾选的 tile 通过 Skip 回调过滤掉，整页不写入 PDF。
	var skipFn func(col, row int) bool
	if len(req.SkippedTiles) > 0 {
		skipSet := make(map[[2]int]struct{}, len(req.SkippedTiles))
		for _, tc := range req.SkippedTiles {
			skipSet[[2]int{tc.Col, tc.Row}] = struct{}{}
		}
		skipFn = func(col, row int) bool {
			_, skip := skipSet[[2]int{col, row}]
			return skip
		}
	}

	// 4. Mode 字符串 → export.Mode 枚举。前端传空 / standard 一律当标准模式处理。
	mode := export.ModeStandard
	if req.Mode == "saving" {
		mode = export.ModeSaving
	}

	useUpscale := req.Upscale &&
		a.secretsCfg != nil &&
		a.secretsCfg.UpscaleReady()

	var upscaleFn export.UpscaleFn
	if useUpscale {
		runner := &upscale.Runner{Secrets: a.secretsCfg}
		upscaleFn = runner.Run
	}
	log.Printf("[export] upscaleRequested=%v upscaleWillRun=%v out=%s", req.Upscale, useUpscale, out)

	// 5. 启动 export.Run；progress 回调把进度透传到前端事件总线。
	res, err := export.Run(export.Request{
		Ctx:            a.ctx,
		Source:         src,
		Crop:           cropRect,
		TargetWMm:      req.TargetWidthCm * 10,
		TargetHMm:      req.TargetHeightCm * 10,
		PaperWMm:       req.PaperWidthMm,
		PaperHMm:       req.PaperHeightMm,
		Landscape:      req.Landscape,
		MarginMm:       req.MarginMm,
		OverlapMm:      req.OverlapMm,
		Mode:           mode,
		AllowRotate:    req.AllowRotate,
		Output:         out,
		Skip:           skipFn,
		UpscaleEnabled: useUpscale,
		Upscale:        upscaleFn,
		Progress: func(stage export.Stage, percent int) {
			wailsruntime.EventsEmit(a.ctx, exportProgressEvent, map[string]any{
				"stage":   string(stage),
				"percent": percent,
				"detail":  export.ProgressDetail(stage, percent),
			})
		},
	})
	if err != nil {
		return nil, err
	}
	return &ExportResponse{OutputPath: res.OutputPath, Pages: res.Pages}, nil
}

// buildPreview 按 previewLongSide 限制尺寸，将图像编码为 PNG 并返回 data URL。
func buildPreview(img image.Image) (string, int, int, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return "", 0, 0, fmt.Errorf("image has zero size")
	}
	pw, ph := w, h
	if w > previewLongSide || h > previewLongSide {
		if w >= h {
			pw = previewLongSide
			ph = int(float64(h) * float64(previewLongSide) / float64(w))
		} else {
			ph = previewLongSide
			pw = int(float64(w) * float64(previewLongSide) / float64(h))
		}
	}
	small := xi.Resize(img, pw, ph, xi.Lanczos)

	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err != nil {
		return "", 0, 0, err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), pw, ph, nil
}
