// Wails Go 绑定的薄封装。
//
// 我们直接通过 window.go.main.App.* 调用，而不是 import wailsjs/go/main，
// 因为 wailsjs 文件在 `wails dev` / `wails build` 时才自动再生；开发阶段
// 绕开它可以避免"编辑过 Go API 但未重启 dev 服务器"时的路径错位问题。

function callApp(method, ...args) {
  const win = /** @type any */ (window);
  const api = win?.go?.main?.App;
  if (!api || typeof api[method] !== 'function') {
    return Promise.reject(
      new Error(`Wails 绑定未就绪：main.App.${method} 不可用，请重启 wails dev`),
    );
  }
  return api[method](...args);
}

// openImageDialog 弹出系统文件选择对话框并加载图片。
// 返回 ImageInfo 或 null（用户取消）。
export function openImageDialog() {
  return callApp('OpenImageDialog');
}

// loadImage 按给定路径加载图片。
export function loadImage(path) {
  return callApp('LoadImage', path);
}

// buildPlan 请求后端计算切分方案。
// req 结构与 Go 端 PlanRequest 对齐。
export function buildPlan(req) {
  return callApp('BuildPlan', req);
}

// getAppFeatures 返回后端能力位（如百度图像增强是否已配置）。
export function getAppFeatures() {
  return callApp('GetAppFeatures');
}

// getBaiduConfig 读取本地百度 AK/SK 配置。
export function getBaiduConfig() {
  return callApp('GetBaiduConfig');
}

// saveBaiduConfig 保存本地百度 AK/SK 配置。
export function saveBaiduConfig(payload) {
  return callApp('SaveBaiduConfig', payload);
}

// exportPdf 触发后端 PDF 导出。内部会弹保存对话框；用户取消时返回 { cancelled: true }。
// req 结构与 Go 端 ExportRequest 对齐。
export function exportPdf(req) {
  return callApp('ExportPDF', req);
}

// onExportProgress 订阅导出进度事件 export:progress。返回一个取消订阅的函数。
// 回调收到 { stage, percent } 形式的对象。
export function onExportProgress(handler) {
  const win = /** @type any */ (window);
  const rt = win?.runtime;
  if (!rt || typeof rt.EventsOn !== 'function') {
    return () => {};
  }
  return rt.EventsOn('export:progress', handler);
}
