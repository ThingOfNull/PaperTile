<script setup>
// App 根组件：持有"一次会话"的全部状态，负责向后端请求切分方案。
// 裁剪采用两阶段模型：pendingCrop（草稿，未生效）+ appliedCrop（已确认，驱动 viewBox 与 PDF 导出）。
// activeCrop（草稿优先、其次已确认）用于驱动实时切分网格，做到所见即所得。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ImageCanvas from './components/ImageCanvas.vue';
import ControlPanel from './components/ControlPanel.vue';
import ExportProgressOverlay from './components/ExportProgressOverlay.vue';
import PreviewGrid from './components/PreviewGrid.vue';
import AboutDialog from './components/AboutDialog.vue';
import BaiduConfigDialog from './components/BaiduConfigDialog.vue';
import {
  exportPdf,
  getAppFeatures,
  getBaiduConfig,
  onExportProgress,
  openImageDialog,
  saveBaiduConfig,
} from './lib/backend';
import { usePlan } from './composables/usePlan';
import { PAPER_PRESETS } from './lib/papers';

const { t } = useI18n();

// ===== 图片与裁剪 =====
const image = ref(null);         // ImageInfo（Go 侧 app.go）
const appliedCrop = ref(null);   // 已确认裁剪（原图像素），驱动 viewBox 与导出
const pendingCrop = ref(null);   // 草稿裁剪（原图像素），仅用于实时预览
const loadError = ref('');

// ===== 纸张与打印参数 =====
const paperKey = ref('a4');
const paperWmm = ref(210);
const paperHmm = ref(297);
const landscape = ref(false);
const marginMm = ref(5);
const overlapMm = ref(0);

// ===== 目标物理尺寸 =====
const targetWcm = ref(0);
const targetHcm = ref(0);
const lockAspect = ref(true);

// ===== 导出模式 =====
const exportMode = ref('standard');
const allowRotate = ref(true); // 省纸模式默认允许 90° 旋转；标准模式被忽略

// ===== 百度清晰度增强（secrets 就绪且裁切满足边长/比例限制才可勾选）=====
const baiduUpscale = ref(false);
const upscaleHint = ref('');
const upscaleEnabled = ref(false);
const baiduConfigured = ref(false);
const baiduConfigVisible = ref(false);
const baiduConfigSaving = ref(false);
const baiduApiKey = ref('');
const baiduSecretKey = ref('');
const baiduConfigPath = ref('config/config.json');

// ===== 视图切换 =====
// 'edit'：裁剪 + 网格；'preview'：2D 拼版预览 + 拣选。未加载图片时强制 edit。
const viewMode = ref('edit');

// ===== 拣选（跳过的图块）=====
// 以 'row,col' 字符串作为 key 存 Set。比数组 O(1) 查询，比对象更贴合"集合"语义。
const skippedTiles = ref(new Set());

function toggleTile({ col, row }) {
  const key = `${row},${col}`;
  const next = new Set(skippedTiles.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  skippedTiles.value = next;
}

function clearSkipped() {
  if (skippedTiles.value.size === 0) return;
  skippedTiles.value = new Set();
}

// 关键参数变化 → 重置拣选状态。PRD v2 §4.5 列的核心维度：
// 纸张 / 方向 / 边距 / 重叠 / 裁剪框 / 目标尺寸。
// 同时 exportMode 切换会让 effectivePaper 在 portrait ↔ landscape 之间切换
// （省纸模式固定 portrait），也会影响 tile 栅格，所以一并纳入。
// 不监听 pendingCrop（草稿不影响 plan）、lockAspect（纯 UI 辅助）、allowRotate（只影响装箱、不改 tile 数量）。
watch(
  [paperWmm, paperHmm, landscape, marginMm, overlapMm, targetWcm, targetHcm, appliedCrop, exportMode],
  () => clearSkipped(),
);

// ===== 衍生值 =====
const effectivePaper = computed(() => {
  if (exportMode.value === 'saving') {
    return { w: paperWmm.value, h: paperHmm.value };
  }
  return landscape.value
    ? { w: paperHmm.value, h: paperWmm.value }
    : { w: paperWmm.value, h: paperHmm.value };
});

// activeCrop 草稿优先，其次已确认。用于实时网格与目标尺寸联动。
const activeCrop = computed(() => pendingCrop.value ?? appliedCrop.value);

const effectiveSourcePx = computed(() => {
  if (!image.value) return null;
  const c = activeCrop.value;
  if (c) return { w: c.x1 - c.x0, h: c.y1 - c.y0 };
  return { w: image.value.width, h: image.value.height };
});

const effectiveAspect = computed(() => {
  const s = effectiveSourcePx.value;
  return s ? s.w / s.h : null;
});

// 与后端 ValidateCropSize 对齐：最长边 ≤5000、最短边 ≥10、长宽比 ≤4:1（体积由后端再压 JPEG）。
const upscaleEligible = computed(() => {
  const s = effectiveSourcePx.value;
  if (!s) return false;
  const short = Math.min(s.w, s.h);
  const long = Math.max(s.w, s.h);
  if (short < 10 || long > 5000) return false;
  if (long / short > 4) return false;
  return true;
});

watch(upscaleEligible, (ok) => {
  if (!ok) upscaleEnabled.value = false;
});

// 只在"预览 + 省纸"两个条件都成立时才让后端顺便跑装箱。
// 这样：1) 编辑模式下主图不需要 packer 结果，主图画面保持轻量；
//      2) 用户点击"预览"时再按需试算，参数抖动期间不反复消耗算力；
//      3) skippedTiles 也仅在 packer 需要参与时才提交，避免无意义的 IPC。
const packingEnabled = computed(
  () => viewMode.value === 'preview' && exportMode.value === 'saving',
);

const planRequest = computed(() => {
  if (!image.value) return null;
  if (effectivePaper.value.w <= 0 || effectivePaper.value.h <= 0) return null;
  if (targetWcm.value <= 0 || targetHcm.value <= 0) return null;
  const req = {
    crop: activeCrop.value,
    targetWidthCm: targetWcm.value,
    targetHeightCm: targetHcm.value,
    paperWidthMm: effectivePaper.value.w,
    paperHeightMm: effectivePaper.value.h,
    marginMm: marginMm.value,
    overlapMm: overlapMm.value,
  };
  if (packingEnabled.value) {
    // skippedTiles 用稳定顺序序列化，避免因 Set 迭代顺序抖动导致 BuildPlan 重复请求。
    const skippedArr = Array.from(skippedTiles.value)
      .map((k) => {
        const [row, col] = k.split(',').map(Number);
        return { row, col };
      })
      .sort((a, b) => a.row - b.row || a.col - b.col);
    req.mode = 'saving';
    req.allowRotate = allowRotate.value;
    req.skippedTiles = skippedArr;
  }
  return req;
});

// 60ms debounce：草稿拖拽过程中实时跟手；BuildPlan 纯数学运算无性能负担。
const { plan, error: planError } = usePlan(planRequest, { debounceMs: 60 });

const warnings = computed(() => {
  const list = [];
  if (plan.value?.warnings) list.push(...plan.value.warnings);
  if (planError.value) list.push(planError.value);
  return list;
});

// ===== 交互：打开图片 =====
async function onOpenImage() {
  loadError.value = '';
  try {
    const info = await openImageDialog();
    if (!info) return;
    applyNewImage(info);
  } catch (e) {
    loadError.value = e?.message ?? String(e);
  }
}

function applyNewImage(info) {
  image.value = info;
  appliedCrop.value = null;
  pendingCrop.value = null;
  resetTargetsFromActive();
}

function resetTargetsFromActive() {
  const img = image.value;
  const s = effectiveSourcePx.value;
  if (!img || !s) return;
  targetWcm.value = +((s.w / img.dpiX) * 2.54).toFixed(2);
  targetHcm.value = +((s.h / img.dpiY) * 2.54).toFixed(2);
}

// ===== 交互：裁剪流程 =====
// 1. 画布拖拽产生 pendingCrop → 草稿 + 实时网格随之更新。
function onUpdatePendingCrop(rect) {
  pendingCrop.value = rect;
  resetTargetsFromActive();
}

// 2. 点击草稿区域（未拖拽）→ 取消草稿，回到已确认状态。
function onCancelPending() {
  pendingCrop.value = null;
  resetTargetsFromActive();
}

// 3. 点击"确认裁剪"→ 草稿升为已确认，SVG viewBox 切换实现"放大"显示。
function onConfirmPending() {
  if (!pendingCrop.value) return;
  appliedCrop.value = pendingCrop.value;
  pendingCrop.value = null;
  resetTargetsFromActive();
}

// 4. 控制面板"重置裁剪"→ 清除所有裁剪状态。
function onResetCrop() {
  appliedCrop.value = null;
  pendingCrop.value = null;
  resetTargetsFromActive();
}

// ===== 目标尺寸联动（锁定宽高比）=====
watch(targetWcm, (nv, ov) => {
  if (!lockAspect.value || !effectiveAspect.value) return;
  if (nv === ov) return;
  const expected = +(nv / effectiveAspect.value).toFixed(2);
  if (Math.abs(expected - targetHcm.value) > 0.01) targetHcm.value = expected;
});
watch(targetHcm, (nv, ov) => {
  if (!lockAspect.value || !effectiveAspect.value) return;
  if (nv === ov) return;
  const expected = +(nv * effectiveAspect.value).toFixed(2);
  if (Math.abs(expected - targetWcm.value) > 0.01) targetWcm.value = expected;
});

// ===== 交互：预设切换 =====
watch(paperKey, (key) => {
  const preset = PAPER_PRESETS.find((p) => p.key === key);
  if (preset && key !== 'custom') {
    paperWmm.value = preset.widthMm;
    paperHmm.value = preset.heightMm;
  }
});

// ===== 导出 PDF =====
// 导出期间：遮罩可见、按钮禁用（ControlPanel 通过 isExporting prop 接收）。
// 进度事件由后端通过 Wails EventsEmit 推送，这里订阅并更新 exportStage/Percent。
const isExporting = ref(false);
const exportStage = ref('');
const exportPercent = ref(0);
const exportDetail = ref('');
const exportMessage = ref(''); // 顶部横幅：成功 / 失败 / 取消
let unsubscribeProgress = null;

// ===== 关于对话框 =====
const showAbout = ref(false);

async function refreshBaiduFeatures() {
  try {
    const [features, cfg] = await Promise.all([
      getAppFeatures(),
      getBaiduConfig(),
    ]);
    baiduUpscale.value = !!features?.baiduUpscale;
    upscaleHint.value = features?.upscaleHint ?? '';
    baiduConfigured.value = !!cfg?.configured;
    baiduApiKey.value = cfg?.apiKey ?? '';
    baiduSecretKey.value = cfg?.secretKey ?? '';
    baiduConfigPath.value = cfg?.configPath ?? 'config/config.json';
  } catch (_) {
    baiduUpscale.value = false;
    upscaleHint.value = '';
    baiduConfigured.value = false;
  }
}

onMounted(() => {
  unsubscribeProgress = onExportProgress((payload) => {
    if (!payload) return;
    exportStage.value = payload.stage ?? '';
    exportPercent.value = Number(payload.percent) || 0;
    exportDetail.value = payload.detail ?? '';
  });
  refreshBaiduFeatures();
});
onBeforeUnmount(() => {
  if (typeof unsubscribeProgress === 'function') unsubscribeProgress();
});

function onOpenBaiduConfig() {
  baiduConfigVisible.value = true;
}

async function onSaveBaiduConfig(payload) {
  if (!payload?.apiKey || !payload?.secretKey || baiduConfigSaving.value) {
    return;
  }
  baiduConfigSaving.value = true;
  try {
    await saveBaiduConfig(payload);
    await refreshBaiduFeatures();
    baiduConfigVisible.value = false;
    exportMessage.value = '百度云配置已保存，本地配置生效。';
  } catch (err) {
    exportMessage.value = t('export.failed', { msg: err?.message ?? String(err) });
  } finally {
    baiduConfigSaving.value = false;
  }
}

async function onExportPdf() {
  if (!image.value || isExporting.value) return;
  if (!planRequest.value) {
    exportMessage.value = t('export.failed', { msg: '参数不完整' });
    return;
  }
  const totalTiles = plan.value?.tiles?.length ?? 0;
  if (totalTiles > 0 && skippedTiles.value.size >= totalTiles) {
    exportMessage.value = t('export.failed', { msg: '所有图块都已取消，至少保留一块再导出' });
    return;
  }
  exportMessage.value = '';
  exportStage.value = '';
  exportPercent.value = 0;
  exportDetail.value = '';
  isExporting.value = true;
  try {
    const skippedArr = Array.from(skippedTiles.value).map((k) => {
      const [row, col] = k.split(',').map(Number);
      return { row, col };
    });
    // 注意：paperWidthMm / paperHeightMm 传 portrait 规范值；方向由 landscape 字段声明。
    // 这与 effectivePaper 的职责不同——effectivePaper 用于切分预览（随 landscape 翻转），
    // 后端则同时需要"规范尺寸 + 方向"才能在省纸模式下做每页独立翻转。
    const req = {
      crop: appliedCrop.value,
      targetWidthCm: targetWcm.value,
      targetHeightCm: targetHcm.value,
      paperWidthMm: paperWmm.value,
      paperHeightMm: paperHmm.value,
      landscape: exportMode.value === 'standard' && landscape.value,
      marginMm: marginMm.value,
      overlapMm: overlapMm.value,
      mode: exportMode.value,
      allowRotate: exportMode.value === 'saving' && allowRotate.value,
      skippedTiles: skippedArr,
      upscale:
        baiduUpscale.value &&
        upscaleEligible.value &&
        upscaleEnabled.value,
    };
    const res = await exportPdf(req);
    if (res?.cancelled) {
      exportMessage.value = t('export.cancelled');
    } else if (res?.outputPath) {
      exportMessage.value = t('export.success', {
        path: res.outputPath,
        pages: res.pages,
      });
    }
  } catch (err) {
    exportMessage.value = t('export.failed', { msg: err?.message ?? String(err) });
  } finally {
    isExporting.value = false;
    exportStage.value = '';
    exportPercent.value = 0;
    exportDetail.value = '';
    setTimeout(() => {
      if (exportMessage.value) exportMessage.value = '';
    }, 5000);
  }
}
</script>

<template>
  <div class="flex h-screen w-screen">
    <!-- 主画布 -->
    <main class="flex-1 flex flex-col min-w-0">
      <!-- 视图切换：仅在有图时显示。切换不清空已有的裁剪 / 拣选状态，仅切换渲染。 -->
      <div
        v-if="image"
        class="flex items-center gap-2 px-4 py-2 border-b border-gray-200 bg-white"
      >
        <div class="inline-flex rounded-md border border-gray-300 overflow-hidden text-sm">
          <button
            class="px-3 py-1 transition-colors"
            :class="viewMode === 'edit'
              ? 'bg-fluent-500 text-white'
              : 'bg-white text-gray-700 hover:bg-gray-50'"
            @click="viewMode = 'edit'"
          >
            编辑
          </button>
          <button
            class="px-3 py-1 border-l border-gray-300 transition-colors"
            :class="viewMode === 'preview'
              ? 'bg-fluent-500 text-white'
              : 'bg-white text-gray-700 hover:bg-gray-50'"
            :disabled="!plan"
            @click="viewMode = 'preview'"
          >
            预览
          </button>
        </div>
        <span v-if="viewMode === 'preview'" class="text-xs text-gray-500">
          点击图块可取消/勾选，参数变更会自动重置选择。
        </span>
      </div>

      <ImageCanvas
        v-if="viewMode === 'edit' || !image"
        class="flex-1"
        :image="image"
        :applied-crop="appliedCrop"
        :pending-crop="pendingCrop"
        :plan="plan"
        :suppress-tile-grid="exportMode === 'saving'"
        @update:pending-crop="onUpdatePendingCrop"
        @cancel-pending="onCancelPending"
        @confirm-pending="onConfirmPending"
        @request-preview="viewMode = 'preview'"
      />
      <PreviewGrid
        v-else
        class="flex-1"
        :image="image"
        :plan="plan"
        :applied-crop="appliedCrop"
        :skipped-tiles="skippedTiles"
        @toggle-tile="toggleTile"
        @clear-skipped="clearSkipped"
      />

      <div
        v-if="loadError"
        class="bg-amber-50 text-amber-700 text-sm px-4 py-2 border-t border-amber-200"
      >
        {{ loadError }}
      </div>
      <div
        v-if="exportMessage"
        class="bg-fluent-50 text-fluent-700 text-sm px-4 py-2 border-t border-fluent-500 whitespace-pre-wrap break-words"
      >
        {{ exportMessage }}
      </div>
    </main>

    <!-- 右侧控制面板 -->
    <ControlPanel
      :image="image"
      v-model:paperKey="paperKey"
      v-model:paperWmm="paperWmm"
      v-model:paperHmm="paperHmm"
      v-model:landscape="landscape"
      v-model:marginMm="marginMm"
      v-model:overlapMm="overlapMm"
      v-model:targetWcm="targetWcm"
      v-model:targetHcm="targetHcm"
      v-model:lockAspect="lockAspect"
      v-model:exportMode="exportMode"
      v-model:allowRotate="allowRotate"
      v-model:upscale-enabled="upscaleEnabled"
      :baidu-upscale="baiduUpscale"
      :baidu-configured="baiduConfigured"
      :upscale-hint="upscaleHint"
      :upscale-eligible="upscaleEligible"
      :warnings="warnings"
      :plan-cols="plan?.cols ?? 0"
      :plan-rows="plan?.rows ?? 0"
      :is-exporting="isExporting"
      @open-image="onOpenImage"
      @reset-crop="onResetCrop"
      @export-pdf="onExportPdf"
      @show-about="showAbout = true"
      @open-baidu-config="onOpenBaiduConfig"
    />

    <!-- 导出进度遮罩 -->
    <ExportProgressOverlay
      :visible="isExporting"
      :stage="exportStage"
      :percent="exportPercent"
      :detail="exportDetail"
    />

    <!-- 关于 -->
    <AboutDialog :visible="showAbout" @close="showAbout = false" />

    <!-- 百度云配置 -->
    <BaiduConfigDialog
      :visible="baiduConfigVisible"
      :initial-api-key="baiduApiKey"
      :initial-secret-key="baiduSecretKey"
      :config-path="baiduConfigPath"
      :saving="baiduConfigSaving"
      @close="baiduConfigVisible = false"
      @save="onSaveBaiduConfig"
    />
  </div>
</template>
