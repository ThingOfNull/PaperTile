package upscale

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"time"

	"go-printtile-pro/internal/export"
	"go-printtile-pro/internal/secrets"
)

// Runner 调用百度智能云「图像清晰度增强」（image_quality_enhance）。
type Runner struct {
	Secrets *secrets.Config
	HTTP    *http.Client
}

func (r *Runner) client() *http.Client {
	if r != nil && r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

// Run 将裁切后的位图编码为 JPEG，经百度 API 增强后再解码为 image.Image。
// progress 会依次触发 StageUploading / StageUpscaling / StageDownloading。
func (r *Runner) Run(ctx context.Context, src image.Image, progress export.ProgressFn) (image.Image, error) {
	if r == nil || r.Secrets == nil || !r.Secrets.UpscaleReady() {
		return nil, fmt.Errorf("upscale: 未配置百度 API 密钥")
	}
	if progress == nil {
		progress = func(export.Stage, int) {}
	}
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if err := ValidateCropSize(w, h); err != nil {
		return nil, err
	}

	hc := r.client()

	progress(export.StageUploading, 8)
	log.Printf("[upscale] encoding jpeg for baidu (%dx%d)", w, h)
	jpegBytes, err := EncodeJPEGUnder(src, MaxJPEGBytesBeforeBase64, 60, 92)
	if err != nil {
		return nil, fmt.Errorf("准备提交图失败（JPEG 编码或体积超限）: %w", err)
	}
	progress(export.StageUploading, 22)

	b64 := base64.StdEncoding.EncodeToString(jpegBytes)
	if len(b64) > MaxBase64EncodedBytes {
		return nil, fmt.Errorf(
			"upscale: Base64 长度 %d 超过百度上限 %d，请缩小裁切区域",
			len(b64),
			MaxBase64EncodedBytes,
		)
	}
	log.Printf("[upscale] jpeg=%d bytes, base64=%d chars", len(jpegBytes), len(b64))

	progress(export.StageUploading, 35)
	tok, err := getBaiduAccessToken(ctx, r.Secrets.Baidu.APIKey, r.Secrets.Baidu.SecretKey, hc)
	if err != nil {
		return nil, fmt.Errorf("获取百度 access_token 失败: %w", err)
	}
	progress(export.StageUploading, 100)

	progress(export.StageUpscaling, 12)
	outBytes, err := callDefinitionEnhance(ctx, tok, b64, hc)
	if err != nil {
		return nil, fmt.Errorf("百度图像清晰度增强失败: %w", err)
	}
	progress(export.StageUpscaling, 100)

	progress(export.StageDownloading, 20)
	img, _, err := image.Decode(bytes.NewReader(outBytes))
	if err != nil {
		return nil, fmt.Errorf("decode enhanced image: %w", err)
	}
	progress(export.StageDownloading, 100)
	log.Printf("[upscale] baidu done, bounds=%v", img.Bounds())
	return img, nil
}
