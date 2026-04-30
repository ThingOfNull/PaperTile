package pdf

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	xi "github.com/disintegration/imaging"
)

// makeSolid 返回一张指定尺寸、指定纯色的 RGBA 图像，供测试使用。
func makeSolid(w, h int, c color.RGBA) image.Image {
	return xi.New(w, h, c)
}

// limitBytes 在断言失败时只输出前 N 字节，避免日志被整个 PDF 淹没。
func limitBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"paper width zero", Config{PaperWMm: 0, PaperHMm: 297, MarginMm: 5}},
		{"paper height negative", Config{PaperWMm: 210, PaperHMm: -10, MarginMm: 5}},
		{"margin negative", Config{PaperWMm: 210, PaperHMm: 297, MarginMm: -1}},
		{"overlap negative", Config{PaperWMm: 210, PaperHMm: 297, MarginMm: 5, OverlapMm: -1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := New(c.cfg); err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
		})
	}
}

func TestAddTilePageGeneratesValidPDF(t *testing.T) {
	b, err := New(Config{PaperWMm: 210, PaperHMm: 297, MarginMm: 5, OverlapMm: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 2 列 × 2 行 = 4 页，覆盖所有 overlap 组合（左/上/两者都有/都没有）。
	tiles := []Tile{
		{Col: 0, Row: 0, WidthMm: 200, HeightMm: 287, HasLeftOverlap: false, HasTopOverlap: false},
		{Col: 1, Row: 0, WidthMm: 200, HeightMm: 287, HasLeftOverlap: true, HasTopOverlap: false},
		{Col: 0, Row: 1, WidthMm: 200, HeightMm: 287, HasLeftOverlap: false, HasTopOverlap: true},
		{Col: 1, Row: 1, WidthMm: 200, HeightMm: 287, HasLeftOverlap: true, HasTopOverlap: true},
	}
	colors := []color.RGBA{
		{R: 255, G: 80, B: 80, A: 255},
		{R: 80, G: 200, B: 80, A: 255},
		{R: 80, G: 120, B: 255, A: 255},
		{R: 240, G: 220, B: 60, A: 255},
	}
	for i := range tiles {
		tiles[i].Image = makeSolid(400, 560, colors[i])
		if err := b.AddTilePage(tiles[i]); err != nil {
			t.Fatalf("AddTilePage[%d]: %v", i, err)
		}
	}

	out := filepath.Join(t.TempDir(), "out.pdf")
	if err := b.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	// 基本健康检查：PDF 魔数、%%EOF 结尾、Pages 树的 Count 字段等于页数。
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output is not a valid PDF (missing %%PDF- header)")
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		t.Fatalf("output is not terminated with %%EOF")
	}
	// gofpdf 把所有页面挂在一棵 Pages 树下，写法形如 "/Type /Pages /Kids [...] /Count 4"。
	// 我们通过 /Count 字段确认页数正确。
	if !bytes.Contains(data, []byte("/Count 4")) {
		t.Fatalf("expected /Count 4 in pages tree, pdf dump: %q", limitBytes(data, 2048))
	}
}

func TestAddPackedPageGeneratesValidPDF(t *testing.T) {
	b, err := New(Config{PaperWMm: 210, PaperHMm: 297, MarginMm: 5})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 两张省纸页：一张 portrait（两块纵向 tile），一张 landscape（一块旋转 tile）。
	mustImg := func(w, h int, c color.RGBA) image.Image { return makeSolid(w, h, c) }
	page1 := PackedPage{
		Landscape: false,
		UsableW:   200,
		UsableH:   287,
		Tiles: []PackedTile{
			{
				Col: 0, Row: 0,
				Image:    mustImg(100, 140, color.RGBA{R: 255, A: 255}),
				WidthMm:  50, HeightMm: 70,
				XMm: 0, YMm: 0,
				PlaceWMm: 50, PlaceHMm: 70,
			},
			{
				Col: 1, Row: 0,
				Image:    mustImg(100, 140, color.RGBA{G: 255, A: 255}),
				WidthMm:  50, HeightMm: 70,
				XMm: 60, YMm: 0,
				PlaceWMm: 50, PlaceHMm: 70,
			},
		},
	}
	page2 := PackedPage{
		Landscape: true,
		UsableW:   287,
		UsableH:   200,
		Tiles: []PackedTile{
			{
				Col: 2, Row: 0,
				Image:    mustImg(100, 140, color.RGBA{B: 255, A: 255}),
				WidthMm:  50, HeightMm: 70,
				XMm: 10, YMm: 10,
				// 旋转：原 50×70 占位变成 70×50
				PlaceWMm: 70, PlaceHMm: 50,
				Rotated:  true,
			},
		},
	}
	if err := b.AddPackedPage(page1); err != nil {
		t.Fatalf("AddPackedPage portrait: %v", err)
	}
	if err := b.AddPackedPage(page2); err != nil {
		t.Fatalf("AddPackedPage landscape: %v", err)
	}
	out := filepath.Join(t.TempDir(), "packed.pdf")
	if err := b.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("not a PDF")
	}
	if !bytes.Contains(data, []byte("/Count 2")) {
		t.Fatalf("expected /Count 2, dump: %q", limitBytes(data, 2048))
	}
}

func TestAddPackedPageRejectsBadInput(t *testing.T) {
	b, err := New(Config{PaperWMm: 210, PaperHMm: 297, MarginMm: 5})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.AddPackedPage(PackedPage{UsableW: 100, UsableH: 100}); err == nil {
		t.Fatalf("expected error for empty tiles")
	}
	if err := b.AddPackedPage(PackedPage{
		UsableW: 0, UsableH: 100,
		Tiles: []PackedTile{{
			Image:    makeSolid(10, 10, color.RGBA{A: 255}),
			PlaceWMm: 10, PlaceHMm: 10,
		}},
	}); err == nil {
		t.Fatalf("expected error for zero usable width")
	}
}

func TestAddTilePageRejectsBadInput(t *testing.T) {
	b, err := New(Config{PaperWMm: 210, PaperHMm: 297, MarginMm: 5})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		name string
		tile Tile
	}{
		{"nil image", Tile{WidthMm: 100, HeightMm: 100}},
		{"zero width", Tile{Image: makeSolid(10, 10, color.RGBA{A: 255}), WidthMm: 0, HeightMm: 100}},
		{"negative height", Tile{Image: makeSolid(10, 10, color.RGBA{A: 255}), WidthMm: 100, HeightMm: -1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := b.AddTilePage(c.tile); err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
		})
	}
}
