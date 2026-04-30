// Package imaging 封装图片加载、DPI 提取与高质量缩放能力。
// DPI 支持来源：PNG 的 pHYs chunk、JPEG 的 JFIF APP0 段。若均无法提取，返回 0 表示缺失。
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"strings"

	// 图像解码器注册（image.Decode 需要）。
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"go-printtile-pro/internal/units"

	xi "github.com/disintegration/imaging"
)

// LoadedImage 保存已解码的原图及其 DPI 元数据。
type LoadedImage struct {
	Image   image.Image // 解码后的原图
	Width   int         // 像素宽
	Height  int         // 像素高
	Format  string      // 图片格式（png/jpeg/webp/...）
	DPIX    int         // 生效 DPI（经 units.NormalizeDPI 处理）
	DPIY    int         // 生效 DPI（经 units.NormalizeDPI 处理）
	RawDPIX int         // 原图元数据中的 DPI，0 表示缺失
	RawDPIY int         // 原图元数据中的 DPI，0 表示缺失
}

// Load 读取指定路径的图片并解析 DPI。
// 当图片无法读取或解码失败时返回错误。DPI 缺失不会报错，会以 0 记录在 RawDPIX/Y。
func Load(path string) (*LoadedImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image file: %w", err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes 从字节流中解码图片。用途同 Load，便于前端传入内存数据或写单测。
func LoadFromBytes(data []byte) (*LoadedImage, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	rawX, rawY := extractDPI(data, format)
	b := img.Bounds()
	return &LoadedImage{
		Image:   img,
		Width:   b.Dx(),
		Height:  b.Dy(),
		Format:  format,
		DPIX:    units.NormalizeDPI(rawX),
		DPIY:    units.NormalizeDPI(rawY),
		RawDPIX: rawX,
		RawDPIY: rawY,
	}, nil
}

// ScaleTo 将图像按 LANCZOS 缩放到指定像素尺寸。
// 当目标尺寸与源图一致时直接返回原图，避免无谓复制。
func ScaleTo(src image.Image, targetW, targetH int) image.Image {
	b := src.Bounds()
	if b.Dx() == targetW && b.Dy() == targetH {
		return src
	}
	return xi.Resize(src, targetW, targetH, xi.Lanczos)
}

// Crop 按像素矩形裁剪图像，使用半开区间 [x0, x1) × [y0, y1)。
// 越界会被自动截断到图像范围内。
func Crop(src image.Image, x0, y0, x1, y1 int) image.Image {
	b := src.Bounds()
	// clamp
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x0 >= x1 || y0 >= y1 {
		return xi.New(1, 1, nil)
	}
	return xi.Crop(src, image.Rect(x0, y0, x1, y1))
}

// extractDPI 根据格式分派到具体的 DPI 解析器。返回 (x, y) DPI，0 表示未解析到。
func extractDPI(data []byte, format string) (int, int) {
	switch strings.ToLower(format) {
	case "png":
		return readPNGDPI(data)
	case "jpeg":
		return readJPEGDPI(data)
	default:
		return 0, 0
	}
}
