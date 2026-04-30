package upscale

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// EncodeJPEGUnder 将图像编码为 JPEG，在 [minQ, maxQ] 内从高到低尝试，直到体积 ≤ maxBytes。
func EncodeJPEGUnder(img image.Image, maxBytes int, minQ, maxQ int) ([]byte, error) {
	if minQ < 1 {
		minQ = 1
	}
	if maxQ > 100 {
		maxQ = 100
	}
	if maxQ < minQ {
		maxQ = minQ
	}
	for q := maxQ; q >= minQ; q -= 4 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, fmt.Errorf("jpeg encode q=%d: %w", q, err)
		}
		if buf.Len() <= maxBytes {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("upscale: jpeg 无法压到 %d 字节以下（请缩小裁切区域）", maxBytes)
}

// ValidateCropSize 校验裁切尺寸满足百度接口边长、长宽比等要求。
func ValidateCropSize(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("upscale: 图像尺寸无效")
	}
	short := w
	long := h
	if w > h {
		short = h
		long = w
	}
	if short < MinInputShortSidePx {
		return fmt.Errorf("upscale: 最短边须 ≥ %d 像素（百度接口限制）", MinInputShortSidePx)
	}
	if long > MaxInputLongSidePx {
		return fmt.Errorf(
			"upscale: 最长边 %d 超过上限 %d 像素（百度接口限制）",
			long,
			MaxInputLongSidePx,
		)
	}
	if float64(long)/float64(short) > float64(MaxAspectRatio)+1e-9 {
		return fmt.Errorf(
			"upscale: 长宽比超过 %d:1（百度接口限制）",
			MaxAspectRatio,
		)
	}
	return nil
}
