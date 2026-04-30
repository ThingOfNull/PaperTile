package upscale

// 百度智能云「图像清晰度增强」接口限制（image_quality_enhance，以官方文档为准）。
const (
	MaxInputLongSidePx       = 5000
	MinInputShortSidePx      = 10
	MaxAspectRatio           = 4
	MaxBase64EncodedBytes    = 10 * 1024 * 1024
	MaxJPEGBytesBeforeBase64 = 7 * 1024 * 1024
)
