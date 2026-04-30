// 预设纸张尺寸（单位：毫米）。
// ISO 216 系列：A3 297×420，A4 210×297，B4 250×353。
// 16K（约 195×270）是国内常见裁切规格。
export const PAPER_PRESETS = [
  { key: 'a4',  labelKey: 'papers.a4',  widthMm: 210, heightMm: 297 },
  { key: 'a3',  labelKey: 'papers.a3',  widthMm: 297, heightMm: 420 },
  { key: 'b4',  labelKey: 'papers.b4',  widthMm: 250, heightMm: 353 },
  { key: 'k16', labelKey: 'papers.k16', widthMm: 195, heightMm: 270 },
  { key: 'custom', labelKey: 'papers.custom', widthMm: 210, heightMm: 297 },
];

// getPaperByKey 按 key 返回预设，未找到时返回 A4。
export function getPaperByKey(key) {
  return PAPER_PRESETS.find((p) => p.key === key) ?? PAPER_PRESETS[0];
}
