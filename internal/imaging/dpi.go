package imaging

import (
	"encoding/binary"
	"math"
)

// pngSignature PNG 文件头的固定 8 字节。
var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// readPNGDPI 从 PNG 字节流中解析 pHYs chunk。
// pHYs chunk 规范：
//   - 4 字节 length（大端）
//   - 4 字节 type "pHYs"
//   - 4 字节 X 方向 pixels-per-unit（大端）
//   - 4 字节 Y 方向 pixels-per-unit（大端）
//   - 1 字节 unit specifier（0 = unknown、1 = meter）
//   - 4 字节 CRC
//
// 当 unit = 1 时按 pixelsPerMeter × 0.0254 换算为 DPI；其他情况返回 0。
func readPNGDPI(data []byte) (int, int) {
	if len(data) < len(pngSignature)+8 {
		return 0, 0
	}
	for i := range pngSignature {
		if data[i] != pngSignature[i] {
			return 0, 0
		}
	}
	offset := len(pngSignature)
	for offset+8 <= len(data) {
		length := binary.BigEndian.Uint32(data[offset : offset+4])
		chunkType := string(data[offset+4 : offset+8])
		dataStart := offset + 8
		dataEnd := dataStart + int(length)
		if dataEnd+4 > len(data) {
			return 0, 0
		}
		if chunkType == "pHYs" && length >= 9 {
			ppuX := binary.BigEndian.Uint32(data[dataStart : dataStart+4])
			ppuY := binary.BigEndian.Uint32(data[dataStart+4 : dataStart+8])
			unit := data[dataStart+8]
			if unit == 1 { // meter
				return ppmToDPI(ppuX), ppmToDPI(ppuY)
			}
			return 0, 0
		}
		if chunkType == "IEND" {
			return 0, 0
		}
		offset = dataEnd + 4 // 跳过 CRC
	}
	return 0, 0
}

// ppmToDPI 将"每米像素数"换算为 DPI，四舍五入取整。
func ppmToDPI(ppm uint32) int {
	if ppm == 0 {
		return 0
	}
	return int(math.Round(float64(ppm) * 0.0254))
}

// readJPEGDPI 从 JPEG 字节流中解析 JFIF APP0 段。
// JFIF APP0 段结构（标记 0xFFE0 之后）：
//   - 2 字节 length（大端，含本身）
//   - 5 字节 identifier "JFIF\0"
//   - 2 字节 version
//   - 1 字节 units（0 = none、1 = dpi、2 = dpcm）
//   - 2 字节 Xdensity（大端）
//   - 2 字节 Ydensity（大端）
//
// 若 units = 1 直接返回 density；units = 2 按 density × 2.54 换算为 DPI。
// 其他情况（包括 EXIF-only JPEG）返回 0。
func readJPEGDPI(data []byte) (int, int) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0
	}
	offset := 2
	for offset+4 <= len(data) {
		if data[offset] != 0xFF {
			return 0, 0
		}
		marker := data[offset+1]
		offset += 2
		// 无长度段：RSTn (0xD0-0xD7)、SOI(0xD8)、EOI(0xD9)。
		if marker == 0xD9 || marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if offset+2 > len(data) {
			return 0, 0
		}
		segLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		segStart := offset + 2
		segEnd := offset + segLen
		if segEnd > len(data) || segLen < 2 {
			return 0, 0
		}
		if marker == 0xE0 && segLen >= 16 {
			// 校验 "JFIF\0" 标识。
			if string(data[segStart:segStart+5]) == "JFIF\x00" {
				units := data[segStart+7]
				xDen := binary.BigEndian.Uint16(data[segStart+8 : segStart+10])
				yDen := binary.BigEndian.Uint16(data[segStart+10 : segStart+12])
				switch units {
				case 1: // pixels per inch
					return int(xDen), int(yDen)
				case 2: // pixels per cm
					return int(math.Round(float64(xDen) * 2.54)),
						int(math.Round(float64(yDen) * 2.54))
				default:
					return 0, 0
				}
			}
		}
		// SOS (0xDA) 之后是图像熵编码数据，不再扫描元数据。
		if marker == 0xDA {
			return 0, 0
		}
		offset = segEnd
	}
	return 0, 0
}
