package export

import "fmt"

// ProgressDetail 返回当前阶段的一句简短说明，供前端进度条副标题展示。
func ProgressDetail(stage Stage, percent int) string {
	switch stage {
	case StageCropping:
		return "正在按裁切区域提取像素…"
	case StageUploading:
		return fmt.Sprintf("正在准备并提交百度图像增强（约 %d%%）…", percent)
	case StageUpscaling:
		return fmt.Sprintf("百度云端处理中（约 %d%%，可能需数十秒）…", percent)
	case StageDownloading:
		return fmt.Sprintf("正在解码增强结果（约 %d%%）…", percent)
	case StageScaling:
		return fmt.Sprintf("正在缩放到目标物理尺寸（约 %d%%）…", percent)
	case StageTiling:
		return "正在计算切分方案…"
	case StageEncoding:
		return fmt.Sprintf("正在编码图块写入 PDF（约 %d%%）…", percent)
	case StageWriting:
		return "正在写入 PDF 文件…"
	case StageCompleted:
		return "完成"
	default:
		return ""
	}
}
