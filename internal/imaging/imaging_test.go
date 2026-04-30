package imaging

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeSolidImage 生成一张纯色小图，用于测试。
func makeSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// encodePNG 编码 PNG（无 pHYs）。
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// injectPNGpHYs 在已编码的 PNG 字节中插入 pHYs chunk。
// ppmX/ppmY 以"每米像素数"为单位；输入 0 表示省略该 chunk。
func injectPNGpHYs(t *testing.T, data []byte, ppmX, ppmY uint32) []byte {
	t.Helper()
	// PNG 签名 8 字节之后第一个 chunk 一定是 IHDR，跳过它再插入 pHYs。
	sigLen := 8
	if len(data) < sigLen+8 {
		t.Fatalf("png too short")
	}
	ihdrLen := binary.BigEndian.Uint32(data[sigLen : sigLen+4])
	ihdrEnd := sigLen + 8 + int(ihdrLen) + 4 // length + type + data + crc

	chunk := buildPHYSChunk(ppmX, ppmY, 1)

	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:ihdrEnd]...)
	out = append(out, chunk...)
	out = append(out, data[ihdrEnd:]...)
	return out
}

// buildPHYSChunk 构造一个合法的 pHYs chunk（含 CRC）。
func buildPHYSChunk(ppmX, ppmY uint32, unit byte) []byte {
	body := make([]byte, 9)
	binary.BigEndian.PutUint32(body[0:4], ppmX)
	binary.BigEndian.PutUint32(body[4:8], ppmY)
	body[8] = unit

	chunk := make([]byte, 0, 4+4+9+4)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, 9)
	chunk = append(chunk, lenBuf...)
	chunk = append(chunk, []byte("pHYs")...)
	chunk = append(chunk, body...)

	h := crc32.NewIEEE()
	h.Write([]byte("pHYs"))
	h.Write(body)
	crcBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBuf, h.Sum32())
	chunk = append(chunk, crcBuf...)
	return chunk
}

func TestLoadPNGWithPHYs(t *testing.T) {
	base := encodePNG(t, makeSolidImage(4, 4, color.White))
	// 11811 ppm ≈ 300 DPI（Photoshop 常见值）。
	data := injectPNGpHYs(t, base, 11811, 11811)

	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.Format != "png" {
		t.Fatalf("format = %q, want png", loaded.Format)
	}
	if loaded.RawDPIX != 300 || loaded.RawDPIY != 300 {
		t.Fatalf("raw DPI = (%d, %d), want (300, 300)", loaded.RawDPIX, loaded.RawDPIY)
	}
	if loaded.DPIX != 300 || loaded.DPIY != 300 {
		t.Fatalf("effective DPI = (%d, %d), want (300, 300)", loaded.DPIX, loaded.DPIY)
	}
}

func TestLoadPNGLowDPIIsRaisedTo300(t *testing.T) {
	base := encodePNG(t, makeSolidImage(4, 4, color.Black))
	// 2835 ppm ≈ 72 DPI。
	data := injectPNGpHYs(t, base, 2835, 2835)

	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.RawDPIX != 72 || loaded.RawDPIY != 72 {
		t.Fatalf("raw DPI = (%d, %d), want (72, 72)", loaded.RawDPIX, loaded.RawDPIY)
	}
	if loaded.DPIX != 300 || loaded.DPIY != 300 {
		t.Fatalf("effective DPI = (%d, %d), want (300, 300)", loaded.DPIX, loaded.DPIY)
	}
}

func TestLoadPNGMissingDPIDefaultsTo300(t *testing.T) {
	// image/png 默认不写入 pHYs。
	data := encodePNG(t, makeSolidImage(4, 4, color.White))
	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.RawDPIX != 0 || loaded.RawDPIY != 0 {
		t.Fatalf("expected raw DPI (0, 0), got (%d, %d)", loaded.RawDPIX, loaded.RawDPIY)
	}
	if loaded.DPIX != 300 || loaded.DPIY != 300 {
		t.Fatalf("expected effective (300, 300), got (%d, %d)", loaded.DPIX, loaded.DPIY)
	}
}

// buildJFIFAPP0 返回 JFIF APP0 段完整字节（含 0xFFE0 标记），单位码与 X/Y density 可配置。
// 总长度固定 20 字节：marker(2) + length(2) + identifier(5) + version(2) + units(1) +
// xDensity(2) + yDensity(2) + thumbW(1) + thumbH(1)。
func buildJFIFAPP0(unit byte, xDen, yDen uint16) []byte {
	seg := make([]byte, 20)
	seg[0] = 0xFF
	seg[1] = 0xE0
	binary.BigEndian.PutUint16(seg[2:4], 16) // length excludes marker, includes length bytes
	copy(seg[4:9], []byte("JFIF\x00"))
	seg[9] = 0x01
	seg[10] = 0x01
	seg[11] = unit
	binary.BigEndian.PutUint16(seg[12:14], xDen)
	binary.BigEndian.PutUint16(seg[14:16], yDen)
	seg[16] = 0
	seg[17] = 0
	seg[18] = 0 // 无缩略图像素数据
	seg[19] = 0
	return seg[:18] // thumbW/thumbH 已在 [16], [17] 表达；后两字节预留无需传
}

// injectJFIFAPP0 在 JPEG SOI 之后插入一段自定义 JFIF APP0。
// Go 1.25 的 image/jpeg 编码器不再写 JFIF，测试里必须手工注入。
func injectJFIFAPP0(t *testing.T, data []byte, unit byte, xDen, yDen uint16) []byte {
	t.Helper()
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatalf("not a JPEG stream")
	}
	seg := buildJFIFAPP0(unit, xDen, yDen)
	out := make([]byte, 0, len(data)+len(seg))
	out = append(out, data[:2]...) // SOI
	out = append(out, seg...)
	out = append(out, data[2:]...)
	return out
}

func TestLoadJPEGStdEncoderMissingDPI(t *testing.T) {
	// Go 1.25 标准库不写 JFIF → 按规范应返回 (0, 0)，即"缺失"。
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, makeSolidImage(8, 8, color.White), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	loaded, err := LoadFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.Format != "jpeg" {
		t.Fatalf("format = %q, want jpeg", loaded.Format)
	}
	if loaded.RawDPIX != 0 || loaded.RawDPIY != 0 {
		t.Fatalf("raw DPI = (%d, %d), want (0, 0)", loaded.RawDPIX, loaded.RawDPIY)
	}
	if loaded.DPIX != 300 || loaded.DPIY != 300 {
		t.Fatalf("effective DPI = (%d, %d), want (300, 300)", loaded.DPIX, loaded.DPIY)
	}
}

func TestLoadJPEGWithManualJFIF150DPI(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, makeSolidImage(8, 8, color.Black), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	data := injectJFIFAPP0(t, buf.Bytes(), 1, 150, 150)
	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.RawDPIX != 150 || loaded.RawDPIY != 150 {
		t.Fatalf("raw DPI = (%d, %d), want (150, 150)", loaded.RawDPIX, loaded.RawDPIY)
	}
	// 150 < 300 → 提升为 300。
	if loaded.DPIX != 300 || loaded.DPIY != 300 {
		t.Fatalf("effective DPI = (%d, %d), want (300, 300)", loaded.DPIX, loaded.DPIY)
	}
}

func TestLoadJPEGWith600DPIKeptAsIs(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, makeSolidImage(8, 8, color.White), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	data := injectJFIFAPP0(t, buf.Bytes(), 1, 600, 600)
	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if loaded.DPIX != 600 || loaded.DPIY != 600 {
		t.Fatalf("effective DPI = (%d, %d), want (600, 600)", loaded.DPIX, loaded.DPIY)
	}
}

func TestLoadJPEGWithDPCMUnits(t *testing.T) {
	// units = 2 表示"每厘米像素数"，200 dpcm ≈ 508 dpi。
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, makeSolidImage(8, 8, color.White), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	data := injectJFIFAPP0(t, buf.Bytes(), 2, 200, 200)
	loaded, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	// 200 * 2.54 = 508
	if loaded.RawDPIX != 508 || loaded.RawDPIY != 508 {
		t.Fatalf("raw DPI = (%d, %d), want (508, 508)", loaded.RawDPIX, loaded.RawDPIY)
	}
	if loaded.DPIX != 508 || loaded.DPIY != 508 {
		t.Fatalf("effective DPI = (%d, %d), want (508, 508)", loaded.DPIX, loaded.DPIY)
	}
}

func TestScaleTo(t *testing.T) {
	src := makeSolidImage(10, 10, color.White)
	dst := ScaleTo(src, 30, 20)
	if b := dst.Bounds(); b.Dx() != 30 || b.Dy() != 20 {
		t.Fatalf("scaled size = %dx%d, want 30x20", b.Dx(), b.Dy())
	}
	// 尺寸一致应复用原图（at least 结果尺寸相同）。
	same := ScaleTo(src, 10, 10)
	if b := same.Bounds(); b.Dx() != 10 || b.Dy() != 10 {
		t.Fatalf("no-op scale unexpected size %dx%d", b.Dx(), b.Dy())
	}
}

func TestCrop(t *testing.T) {
	src := makeSolidImage(10, 10, color.White)
	c := Crop(src, 2, 3, 8, 9)
	if b := c.Bounds(); b.Dx() != 6 || b.Dy() != 6 {
		t.Fatalf("crop size = %dx%d, want 6x6", b.Dx(), b.Dy())
	}
	// 越界裁剪应截断到源范围。
	c2 := Crop(src, -5, -5, 20, 20)
	if b := c2.Bounds(); b.Dx() != 10 || b.Dy() != 10 {
		t.Fatalf("clamped crop size = %dx%d, want 10x10", b.Dx(), b.Dy())
	}
}
