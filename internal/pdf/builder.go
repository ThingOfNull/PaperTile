// Package pdf 封装 phpdave11/gofpdf，提供两种"把图块写成一页"的高层 API。
//
//	- AddTilePage  →  标准模式：一页一块，方向固定（cfg.Landscape）。绘制图块 + 重叠底色 +
//	  裁切十字 + 坐标水印。
//	- AddPackedPage → 省纸模式：一页多块，方向按页决定（由 packer 计算好）。每块可独立
//	  旋转 90°；绘制时把图像预旋转后贴图，避免 PDF 层矩阵变换的尺寸/分辨率陷阱。
//
// 纸张坐标系：原点左上角，X 向右、Y 向下，单位均为毫米（mm）。
// cfg.PaperWMm × cfg.PaperHMm 必须传 portrait 规范尺寸，横向由 Landscape 字段决定。
package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"

	xi "github.com/disintegration/imaging"
	"github.com/phpdave11/gofpdf"
)

// JPEG 编码质量。92 与原版 ReportLab 默认值相当，体积/画质折衷合适。
const jpegQuality = 92

// 裁切十字标记：线段长度与线宽。5 mm 默认边距下，3 mm 线段能完全落在边距内且清晰可见。
const (
	cropMarkLengthMm = 3.0
	cropMarkLineMm   = 0.15
)

// Config 一次 PDF 生成的全局参数。所有字段单位均为毫米。
// PaperWMm × PaperHMm 必须是 portrait 规范尺寸（A4 即 210 × 297）；
// 标准模式下 Landscape=true 会让每页都翻成横向。省纸模式由 AddPackedPage 自行决定方向。
type Config struct {
	PaperWMm  float64 // 纸张规范宽（portrait）
	PaperHMm  float64 // 纸张规范高（portrait）
	MarginMm  float64 // 打印边距（四周等宽）
	OverlapMm float64 // 标准模式下相邻图块的重叠量，用于画半透明底色；0 = 不绘制
	Landscape bool    // 标准模式默认方向；省纸模式无视此字段
}

// Tile 标准模式下的单块图块。Image 为已裁剪到该图块范围的位图，
// 像素尺寸应与 WidthMm / HeightMm 严格对应。
type Tile struct {
	Col            int
	Row            int
	Image          image.Image
	WidthMm        float64
	HeightMm       float64
	HasLeftOverlap bool
	HasTopOverlap  bool
}

// PackedTile 省纸模式下一个已被 packer 定位的图块。
//
//	XMm / YMm 的原点取"可用区左上"（= 已减过 margin），避免调用方再算一次 +m。
//	Rotated=true 时 PlaceWMm × PlaceHMm = 原始 HeightMm × WidthMm。
//	绘制阶段通过 xi.Rotate90 预旋转位图，之后按 PlaceWMm × PlaceHMm 贴图。
type PackedTile struct {
	Col, Row   int
	Image      image.Image
	WidthMm    float64 // 原始宽（未旋转）
	HeightMm   float64 // 原始高
	XMm, YMm   float64 // 相对"可用区左上"的位置
	PlaceWMm   float64 // 实际占位宽（= 旋转后的宽）
	PlaceHMm   float64 // 实际占位高
	Rotated    bool
}

// PackedPage 一张省纸模式 PDF 页面。Landscape=true → 对应 landscape 纸张。
// UsableW / UsableH 是"已去掉 margin 的可用区域"在**该页面当前方向下**的宽高。
type PackedPage struct {
	Landscape bool
	UsableW   float64
	UsableH   float64
	Tiles     []PackedTile
}

// Builder 流式 PDF 构建器。线程不安全，同一实例不能并发调用。
type Builder struct {
	pdf *gofpdf.Fpdf
	cfg Config
}

// New 创建与给定配置匹配的 Builder。gofpdf 以 portrait 规范尺寸初始化，
// 每次 AddPageFormat 再按需指定方向，兼容标准 / 省纸两种流程。
func New(cfg Config) (*Builder, error) {
	if cfg.PaperWMm <= 0 || cfg.PaperHMm <= 0 {
		return nil, fmt.Errorf("pdf: paper size must be positive (got %vx%v)", cfg.PaperWMm, cfg.PaperHMm)
	}
	if cfg.MarginMm < 0 {
		return nil, fmt.Errorf("pdf: margin must be non-negative (got %v)", cfg.MarginMm)
	}
	if cfg.OverlapMm < 0 {
		return nil, fmt.Errorf("pdf: overlap must be non-negative (got %v)", cfg.OverlapMm)
	}

	doc := gofpdf.NewCustom(&gofpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		Size:           gofpdf.SizeType{Wd: cfg.PaperWMm, Ht: cfg.PaperHMm},
	})
	doc.SetAutoPageBreak(false, 0)
	doc.SetMargins(0, 0, 0)
	doc.SetCompression(true)
	doc.SetCreator("Go-PrintTile Pro", true)

	return &Builder{pdf: doc, cfg: cfg}, nil
}

// orient 把 bool 转成 gofpdf 认识的 "P"/"L"。
func orient(landscape bool) string {
	if landscape {
		return "L"
	}
	return "P"
}

// paperSize 返回 gofpdf 初始化时约定的 portrait 规范尺寸。AddPageFormat 会根据 orientation
// 自动决定是否内部交换，所以此处始终返回 portrait。
func (b *Builder) paperSize() gofpdf.SizeType {
	return gofpdf.SizeType{Wd: b.cfg.PaperWMm, Ht: b.cfg.PaperHMm}
}

// AddTilePage 写入一张 tile 对应的 PDF 页（标准模式）。
// 顺序：位图 → 重叠半透明底色 → 裁切十字 → 坐标水印。
func (b *Builder) AddTilePage(t Tile) error {
	if err := b.pdf.Error(); err != nil {
		return err
	}
	if t.Image == nil {
		return fmt.Errorf("pdf: tile image is nil")
	}
	if t.WidthMm <= 0 || t.HeightMm <= 0 {
		return fmt.Errorf("pdf: tile physical size must be positive (got %vx%v)", t.WidthMm, t.HeightMm)
	}

	b.pdf.AddPageFormat(orient(b.cfg.Landscape), b.paperSize())
	m := b.cfg.MarginMm

	if err := b.drawTileImage(t.Image, fmt.Sprintf("tile_%d_%d", t.Col, t.Row), m, m, t.WidthMm, t.HeightMm); err != nil {
		return err
	}
	if b.cfg.OverlapMm > 0 {
		b.drawOverlapBands(t, m)
	}
	b.drawCropMarks(m, m, t.WidthMm, t.HeightMm)
	b.drawCoordWatermark(t.Row, t.Col, false, m+1.5, m-1.2)

	return b.pdf.Error()
}

// AddPackedPage 写入一张多块页（省纸模式）。每块独立定位、可旋转。
// 省纸模式下 tile 相对独立、相邻 tile 不共享物理边界，故不绘制 overlap 底色。
// 裁切十字画在**每块 tile 自身四角**，让拼接时对齐更直观。
func (b *Builder) AddPackedPage(p PackedPage) error {
	if err := b.pdf.Error(); err != nil {
		return err
	}
	if len(p.Tiles) == 0 {
		return fmt.Errorf("pdf: packed page has zero tiles")
	}
	if p.UsableW <= 0 || p.UsableH <= 0 {
		return fmt.Errorf("pdf: packed page usable area must be positive (got %vx%v)", p.UsableW, p.UsableH)
	}

	b.pdf.AddPageFormat(orient(p.Landscape), b.paperSize())
	m := b.cfg.MarginMm

	for _, t := range p.Tiles {
		if t.Image == nil {
			return fmt.Errorf("pdf: packed tile image is nil (R%d-C%d)", t.Row+1, t.Col+1)
		}
		if t.PlaceWMm <= 0 || t.PlaceHMm <= 0 {
			return fmt.Errorf("pdf: packed tile placement size must be positive (got %vx%v)", t.PlaceWMm, t.PlaceHMm)
		}
		// 预旋转：把 image 在 CPU 侧旋 90° 顺时针；这样贴图时不需要 PDF 矩阵变换，
		// 也避免 gofpdf 在 transform 下估算不准图像分辨率导致的模糊。
		img := t.Image
		if t.Rotated {
			img = xi.Rotate90(img)
			// 180° 还是 90° CCW 视感上等价（方形旋转 90° 有两种方向）。这里固定 CCW
			// 并不是必需的；但 Rotate90 是 CCW，我们在水印上标 "↻90°" 时以"用户
			// 看上去顺时针扭了" 为准：在"↻90°" 下应手动把纸拧回去，两种方向对应
			// 同一拼接结果（图块是矩形、无方向感）。
		}
		name := fmt.Sprintf("ptile_%d_%d", t.Col, t.Row)
		if err := b.drawTileImage(img, name, m+t.XMm, m+t.YMm, t.PlaceWMm, t.PlaceHMm); err != nil {
			return err
		}
		b.drawCropMarks(m+t.XMm, m+t.YMm, t.PlaceWMm, t.PlaceHMm)
		b.drawCoordWatermark(t.Row, t.Col, t.Rotated,
			m+t.XMm+1.5,
			// 水印画在 tile 内部左上，避免与相邻 tile 的水印挤在一起。
			m+t.YMm+3.5)
	}

	return b.pdf.Error()
}

// Save 把累积的页面写出到 path。若写入失败，会尝试删除部分文件避免留下残缺 PDF。
func (b *Builder) Save(path string) error {
	if err := b.pdf.Error(); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("pdf: create %s: %w", path, err)
	}
	writeErr := b.pdf.Output(f)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("pdf: write: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("pdf: close: %w", closeErr)
	}
	return b.pdf.Error()
}

// drawTileImage 把位图编码为 JPEG 并在 (x, y) 位置以 (w, h) 物理尺寸绘制。
func (b *Builder) drawTileImage(img image.Image, name string, x, y, w, h float64) error {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return fmt.Errorf("pdf: encode %s: %w", name, err)
	}
	opts := gofpdf.ImageOptions{ImageType: "JPG", ReadDpi: false}
	b.pdf.RegisterImageOptionsReader(name, opts, &buf)
	if err := b.pdf.Error(); err != nil {
		return err
	}
	b.pdf.ImageOptions(name, x, y, w, h, false, opts, 0, "")
	return b.pdf.Error()
}

// drawOverlapBands 标准模式专用：在 tile 的左侧 / 顶部画一条半透明琥珀色横条。
// 仅绘制"左 / 上"，避免相邻两页在拼接时重复绘制同一区域。
func (b *Builder) drawOverlapBands(t Tile, m float64) {
	b.pdf.SetAlpha(0.22, "Normal")
	b.pdf.SetFillColor(255, 196, 0)
	if t.HasLeftOverlap {
		w := b.cfg.OverlapMm
		if w > t.WidthMm {
			w = t.WidthMm
		}
		b.pdf.Rect(m, m, w, t.HeightMm, "F")
	}
	if t.HasTopOverlap {
		h := b.cfg.OverlapMm
		if h > t.HeightMm {
			h = t.HeightMm
		}
		b.pdf.Rect(m, m, t.WidthMm, h, "F")
	}
	b.pdf.SetAlpha(1.0, "Normal")
}

// drawCropMarks 在以 (x, y) 为左上角、宽高 w × h 的矩形四角各画一个细十字。
// 公用给标准模式（tile 占满可用区）与省纸模式（每块 tile 自身四角）。
func (b *Builder) drawCropMarks(x, y, w, h float64) {
	b.pdf.SetLineWidth(cropMarkLineMm)
	b.pdf.SetDrawColor(0, 0, 0)
	corners := [4][2]float64{
		{x, y},
		{x + w, y},
		{x, y + h},
		{x + w, y + h},
	}
	for _, c := range corners {
		cx, cy := c[0], c[1]
		b.pdf.Line(cx-cropMarkLengthMm, cy, cx+cropMarkLengthMm, cy)
		b.pdf.Line(cx, cy-cropMarkLengthMm, cx, cy+cropMarkLengthMm)
	}
}

// drawCoordWatermark 打印 "R{row+1}-C{col+1}"（如有旋转则追加 "↻"）。
// 文字基线由调用方决定；Fluent 蓝色做视觉锚点，Helvetica 内置字体避免中文字体打包。
// baselineX/Y 是文本基线坐标。
func (b *Builder) drawCoordWatermark(row, col int, rotated bool, baselineX, baselineY float64) {
	b.pdf.SetFont("Helvetica", "B", 8)
	b.pdf.SetTextColor(0, 120, 212)
	text := fmt.Sprintf("R%d-C%d", row+1, col+1)
	if rotated {
		// Helvetica 内置字体不含 ↻，退而求其次用 " R" 后缀（Rotated）。
		text += " R"
	}
	// 防御：baselineY < 3mm 时极端纸张上会被裁掉，强制下移。
	if baselineY < 3.0 {
		baselineY = 3.0
	}
	b.pdf.Text(baselineX, baselineY, text)
	b.pdf.SetTextColor(0, 0, 0)
}

// 保证 io.Writer 接口签名后续变更能被静态检查到。
var _ io.Writer = (*bytes.Buffer)(nil)
