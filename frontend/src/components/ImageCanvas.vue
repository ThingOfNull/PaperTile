<script setup>
// ImageCanvas 主画布：预览 + 两阶段裁剪 + 实时切分网格 + 放大镜。
//
// 坐标系约定：
//   - 原图像素 (image px)：image.width × image.height，appliedCrop / pendingCrop 用这套坐标存储。
//   - 预览像素 (preview px)：image.previewWidth × image.previewHeight，也是 SVG viewBox 的坐标空间。
//   - 屏幕像素 (screen px)：DOM / CSS 像素，用于定位 HTML 浮层（按钮、放大镜）。
//
// 裁剪流程（两阶段）：
//   1. 拖拽鼠标 → 产生 pendingCrop（草稿，绿色高亮 + 虚线）；
//   2. 在草稿上单击 → 取消草稿（不影响 appliedCrop）；
//   3. 点击"确认裁剪"按钮 → pendingCrop 变为 appliedCrop，SVG viewBox 切到该区域，画布"放大"为裁剪后视图；
//   4. 顶部「重置裁剪」按钮清空 appliedCrop 回到整图。
//
// 放大镜：固定右上角 168×168，4× 放大预览图的光标附近区域，红色十字线；
//   为保证精度已足够，不额外从后端取高分辨率补丁。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';

const props = defineProps({
  image: { type: Object, default: null },
  appliedCrop: { type: Object, default: null },
  pendingCrop: { type: Object, default: null },
  plan: { type: Object, default: null },
  // 省纸模式下主图裁切示意图没有明确含义（tile 实际会被打乱 / 旋转后放到不同页面上）。
  // 此时由父组件把 suppressTileGrid 置为 true，ImageCanvas 不绘制任何 tile 网格，
  // 仅在画布底部显示"请切换预览试算"提示，引导用户点到预览视图。
  suppressTileGrid: { type: Boolean, default: false },
});
const emit = defineEmits(['update:pendingCrop', 'cancel-pending', 'confirm-pending', 'request-preview']);

// ===== 常量 =====
const SNAP_CSS_PX = 12;       // 边缘吸附阈值（屏幕像素）。12px ≈ 手感刚刚好，再大就"误吸"。
const DRAG_THRESHOLD_CSS = 4; // 视为"拖拽"的最小位移（屏幕像素）
const MAG_SIZE = 168;         // 放大镜边长（CSS 像素）
const MAG_ZOOM = 4;           // 放大倍数
// 画布四周的呼吸留白。图片与窗口边缘之间必须留缓冲区，否则：
//   ① 光标移到窗口边缘时已经顶出图片外，吸附根本来不及触发；
//   ② 视觉上图片"卡死"在窗口边，压抑。
const CANVAS_PAD_PX = 24;

// ===== DOM 引用 & 视口尺寸 =====
const container = ref(null);
const svgRef = ref(null);
const containerSize = ref({ w: 0, h: 0 });
let resizeObserver = null;

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    if (!container.value) return;
    const r = container.value.getBoundingClientRect();
    containerSize.value = { w: r.width, h: r.height };
  });
  resizeObserver.observe(container.value);
});
onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});

// ===== 坐标辅助：原图 ↔ 预览 =====
function imgToPreview(rect) {
  const img = props.image;
  if (!img || !rect) return null;
  const k = img.previewWidth / img.width;
  return {
    x: rect.x0 * k,
    y: rect.y0 * k,
    w: (rect.x1 - rect.x0) * k,
    h: (rect.y1 - rect.y0) * k,
  };
}
function previewToImg(pxRect) {
  const img = props.image;
  if (!img || !pxRect) return null;
  const k = img.width / img.previewWidth;
  const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
  return {
    x0: Math.round(clamp(pxRect.x * k, 0, img.width)),
    y0: Math.round(clamp(pxRect.y * k, 0, img.height)),
    x1: Math.round(clamp((pxRect.x + pxRect.w) * k, 0, img.width)),
    y1: Math.round(clamp((pxRect.y + pxRect.h) * k, 0, img.height)),
  };
}

// ===== viewBox：整图模式 vs 裁剪后放大模式 =====
// 未裁剪时为整张预览；裁剪后 viewBox 切到 appliedCrop 对应的预览区域，
// SVG preserveAspectRatio=meet 会自动把该区域等比填满 viewport，效果等同"放大显示裁剪区"。
const viewBox = computed(() => {
  const img = props.image;
  if (!img) return { x: 0, y: 0, w: 1, h: 1 };
  const applied = imgToPreview(props.appliedCrop);
  if (applied) return { x: applied.x, y: applied.y, w: applied.w, h: applied.h };
  return { x: 0, y: 0, w: img.previewWidth, h: img.previewHeight };
});
const viewBoxAttr = computed(
  () => `${viewBox.value.x} ${viewBox.value.y} ${viewBox.value.w} ${viewBox.value.h}`,
);

// SVG 实际渲染尺寸（去掉四周留白）。pxPerUnit 必须基于它而不是 container，
// 否则吸附阈值换算会偏大，手感会"迟钝"。
const svgSize = computed(() => ({
  w: Math.max(0, containerSize.value.w - 2 * CANVAS_PAD_PX),
  h: Math.max(0, containerSize.value.h - 2 * CANVAS_PAD_PX),
}));

// pxPerUnit 每 1 个 viewBox 单位（= 1 个预览像素）换算到多少屏幕像素。
// 基于 preserveAspectRatio=meet 的实际渲染缩放。
const pxPerUnit = computed(() => {
  const { w, h } = svgSize.value;
  const vb = viewBox.value;
  if (!w || !h || !vb.w || !vb.h) return 1;
  return Math.min(w / vb.w, h / vb.h);
});

// ===== 鼠标→viewBox 坐标（含边缘吸附 + 视口钳制）=====
function screenToViewBox(e) {
  const svg = svgRef.value;
  if (!svg) return { x: 0, y: 0 };
  const pt = svg.createSVGPoint();
  pt.x = e.clientX;
  pt.y = e.clientY;
  const ctm = svg.getScreenCTM();
  if (!ctm) return { x: 0, y: 0 };
  const p = pt.matrixTransform(ctm.inverse());
  return snapAndClamp(p.x, p.y);
}
function snapAndClamp(x, y) {
  const vb = viewBox.value;
  const snapUnit = SNAP_CSS_PX / Math.max(0.0001, pxPerUnit.value);
  if (Math.abs(x - vb.x) < snapUnit) x = vb.x;
  if (Math.abs(x - (vb.x + vb.w)) < snapUnit) x = vb.x + vb.w;
  if (Math.abs(y - vb.y) < snapUnit) y = vb.y;
  if (Math.abs(y - (vb.y + vb.h)) < snapUnit) y = vb.y + vb.h;
  x = Math.max(vb.x, Math.min(vb.x + vb.w, x));
  y = Math.max(vb.y, Math.min(vb.y + vb.h, y));
  return { x, y };
}

// ===== 拖拽状态 =====
const dragStart = ref(null); // 预览像素
const dragCur = ref(null);   // 预览像素
const dragged = ref(false);

function onSvgPointerDown(e) {
  if (!props.image) return;
  const p = screenToViewBox(e);
  dragStart.value = p;
  dragCur.value = p;
  dragged.value = false;
  svgRef.value.setPointerCapture(e.pointerId);
}
function onSvgPointerMove(e) {
  updateMagnifier(e);
  if (!dragStart.value) return;
  const p = screenToViewBox(e);
  dragCur.value = p;
  const thUnit = DRAG_THRESHOLD_CSS / Math.max(0.0001, pxPerUnit.value);
  if (
    Math.abs(p.x - dragStart.value.x) > thUnit ||
    Math.abs(p.y - dragStart.value.y) > thUnit
  ) {
    dragged.value = true;
  }
}
function onSvgPointerUp(e) {
  if (!dragStart.value) return;
  const start = dragStart.value;
  const end = dragCur.value;
  const wasDragged = dragged.value;
  dragStart.value = null;
  dragCur.value = null;
  dragged.value = false;
  try {
    svgRef.value.releasePointerCapture(e.pointerId);
  } catch (_) {
    /* ignore */
  }
  if (!wasDragged) {
    // 点击（未拖拽）：若点在当前草稿内，视为"取消草稿"。
    const pendingP = imgToPreview(props.pendingCrop);
    if (pendingP && pointInRect(start, pendingP)) {
      emit('cancel-pending');
    }
    return;
  }
  // 拖拽完成：组成新的 pendingCrop。
  const pxRect = {
    x: Math.min(start.x, end.x),
    y: Math.min(start.y, end.y),
    w: Math.abs(end.x - start.x),
    h: Math.abs(end.y - start.y),
  };
  if (pxRect.w < 1 || pxRect.h < 1) return;
  const imgRect = previewToImg(pxRect);
  if (imgRect.x1 - imgRect.x0 < 1 || imgRect.y1 - imgRect.y0 < 1) return;
  emit('update:pendingCrop', imgRect);
}
function pointInRect(p, r) {
  return p.x >= r.x && p.x <= r.x + r.w && p.y >= r.y && p.y <= r.y + r.h;
}

// ===== 渲染：已确认 / 草稿矩形（预览像素）=====
const appliedInPreview = computed(() => imgToPreview(props.appliedCrop));
const pendingInPreview = computed(() => imgToPreview(props.pendingCrop));

// 活跃裁剪区域 = 草稿优先，其次已确认；用于约束网格范围。
const activeRectInPreview = computed(() => {
  if (pendingInPreview.value) return pendingInPreview.value;
  if (appliedInPreview.value) return appliedInPreview.value;
  const img = props.image;
  return img ? { x: 0, y: 0, w: img.previewWidth, h: img.previewHeight } : null;
});

// 拖拽中实时绘制的矩形
const dragRect = computed(() => {
  if (!dragStart.value || !dragCur.value || !dragged.value) return null;
  return {
    x: Math.min(dragStart.value.x, dragCur.value.x),
    y: Math.min(dragStart.value.y, dragCur.value.y),
    w: Math.abs(dragCur.value.x - dragStart.value.x),
    h: Math.abs(dragCur.value.y - dragStart.value.y),
  };
});

// ===== 网格线（预览像素）=====
const gridLines = computed(() => {
  const p = props.plan;
  const active = activeRectInPreview.value;
  // suppressTileGrid 或缺少 plan 均返回空集合；模板 v-for 自动跳过。
  if (props.suppressTileGrid || !p || !active || !p.cols || !p.rows) {
    return { vertical: [], horizontal: [], tiles: [] };
  }
  const fx = active.w / p.sourceW;
  const fy = active.h / p.sourceH;
  const vertical = [];
  const horizontal = [];
  for (let i = 1; i < p.cols; i++) {
    vertical.push(active.x + i * p.stepPxX * fx);
    if (p.overlapPxX > 0) {
      vertical.push(active.x + (i * p.stepPxX + p.overlapPxX) * fx);
    }
  }
  for (let j = 1; j < p.rows; j++) {
    horizontal.push(active.y + j * p.stepPxY * fy);
    if (p.overlapPxY > 0) {
      horizontal.push(active.y + (j * p.stepPxY + p.overlapPxY) * fy);
    }
  }
  const tiles = (p.tiles || []).map((t) => ({
    col: t.col,
    row: t.row,
    x: active.x + t.x0 * fx,
    y: active.y + t.y0 * fy,
    w: (t.x1 - t.x0) * fx,
    h: (t.y1 - t.y0) * fy,
  }));
  return { vertical, horizontal, tiles };
});

// ===== 确认/取消按钮浮层位置（屏幕像素，相对 container）=====
function viewBoxPointToScreen(vx, vy) {
  const svg = svgRef.value;
  if (!svg || !container.value) return null;
  const pt = svg.createSVGPoint();
  pt.x = vx;
  pt.y = vy;
  const ctm = svg.getScreenCTM();
  if (!ctm) return null;
  const screen = pt.matrixTransform(ctm);
  const cr = container.value.getBoundingClientRect();
  return { left: screen.x - cr.left, top: screen.y - cr.top };
}
const confirmButtonsPos = computed(() => {
  if (!pendingInPreview.value) return null;
  // 依赖 containerSize 以在尺寸变化时重新计算
  void containerSize.value;
  const r = pendingInPreview.value;
  return viewBoxPointToScreen(r.x + r.w, r.y + r.h);
});

// ===== 放大镜 =====
const magnifierVisible = ref(false);
const magnifierPx = ref({ x: 0, y: 0 }); // 预览像素坐标
function updateMagnifier(e) {
  if (!props.image) {
    magnifierVisible.value = false;
    return;
  }
  const p = screenToViewBoxNoSnap(e); // 放大镜不应用吸附，避免跳跃感
  const img = props.image;
  if (p.x < 0 || p.y < 0 || p.x > img.previewWidth || p.y > img.previewHeight) {
    magnifierVisible.value = false;
    return;
  }
  magnifierVisible.value = true;
  magnifierPx.value = p;
}
function screenToViewBoxNoSnap(e) {
  const svg = svgRef.value;
  if (!svg) return { x: 0, y: 0 };
  const pt = svg.createSVGPoint();
  pt.x = e.clientX;
  pt.y = e.clientY;
  const ctm = svg.getScreenCTM();
  if (!ctm) return { x: 0, y: 0 };
  const p = pt.matrixTransform(ctm.inverse());
  return { x: p.x, y: p.y };
}
function onSvgPointerLeave() {
  magnifierVisible.value = false;
}

const magnifierStyle = computed(() => {
  const img = props.image;
  if (!img) return {};
  const bgW = img.previewWidth * MAG_ZOOM;
  const bgH = img.previewHeight * MAG_ZOOM;
  const half = MAG_SIZE / 2;
  const posX = -(magnifierPx.value.x * MAG_ZOOM - half);
  const posY = -(magnifierPx.value.y * MAG_ZOOM - half);
  return {
    width: `${MAG_SIZE}px`,
    height: `${MAG_SIZE}px`,
    backgroundImage: `url(${img.previewDataUrl})`,
    backgroundRepeat: 'no-repeat',
    backgroundSize: `${bgW}px ${bgH}px`,
    backgroundPosition: `${posX}px ${posY}px`,
  };
});

// 当 appliedCrop / pendingCrop 变化时，下一帧重算按钮位置（DOM 还未更新）。
watch(
  [() => props.appliedCrop, () => props.pendingCrop, containerSize],
  async () => {
    await nextTick();
  },
  { deep: true },
);
</script>

<template>
  <div ref="container" class="relative w-full h-full overflow-hidden bg-slate-100">
    <svg
      v-if="image && svgSize.w > 0 && svgSize.h > 0"
      ref="svgRef"
      class="absolute touch-none select-none"
      :class="pendingCrop ? 'cursor-default' : 'cursor-crosshair'"
      :style="{
        left: CANVAS_PAD_PX + 'px',
        top: CANVAS_PAD_PX + 'px',
        width: svgSize.w + 'px',
        height: svgSize.h + 'px',
      }"
      :width="svgSize.w"
      :height="svgSize.h"
      :viewBox="viewBoxAttr"
      preserveAspectRatio="xMidYMid meet"
      @pointerdown="onSvgPointerDown"
      @pointermove="onSvgPointerMove"
      @pointerup="onSvgPointerUp"
      @pointercancel="onSvgPointerUp"
      @pointerleave="onSvgPointerLeave"
    >
      <!-- 预览图（SVG <image>，便于 viewBox 直接裁剪显示） -->
      <image
        :href="image.previewDataUrl"
        x="0"
        y="0"
        :width="image.previewWidth"
        :height="image.previewHeight"
      />

      <!-- 整图模式下，已确认裁剪矩形用柔和蓝框示意 -->
      <rect
        v-if="appliedInPreview && !pendingCrop"
        :x="appliedInPreview.x"
        :y="appliedInPreview.y"
        :width="appliedInPreview.w"
        :height="appliedInPreview.h"
        fill="none"
        stroke="#0078d4"
        stroke-width="2"
        vector-effect="non-scaling-stroke"
      />

      <!-- 草稿矩形：高亮绿色 + 虚线 -->
      <rect
        v-if="pendingInPreview"
        :x="pendingInPreview.x"
        :y="pendingInPreview.y"
        :width="pendingInPreview.w"
        :height="pendingInPreview.h"
        fill="rgba(34, 197, 94, 0.10)"
        stroke="#16a34a"
        stroke-width="2"
        stroke-dasharray="6 3"
        vector-effect="non-scaling-stroke"
      />

      <!-- 网格虚线（受 activeRect 范围约束） -->
      <line
        v-for="(y, i) in gridLines.horizontal"
        :key="'h' + i"
        :x1="activeRectInPreview?.x ?? 0"
        :x2="(activeRectInPreview?.x ?? 0) + (activeRectInPreview?.w ?? 0)"
        :y1="y"
        :y2="y"
        stroke="#0078d4"
        stroke-width="1"
        stroke-dasharray="6 4"
        vector-effect="non-scaling-stroke"
      />
      <line
        v-for="(x, i) in gridLines.vertical"
        :key="'v' + i"
        :x1="x"
        :x2="x"
        :y1="activeRectInPreview?.y ?? 0"
        :y2="(activeRectInPreview?.y ?? 0) + (activeRectInPreview?.h ?? 0)"
        stroke="#0078d4"
        stroke-width="1"
        stroke-dasharray="6 4"
        vector-effect="non-scaling-stroke"
      />

      <!-- 坐标水印 -->
      <g v-for="t in gridLines.tiles" :key="`t-${t.col}-${t.row}`">
        <text
          :x="t.x + 4"
          :y="t.y + 14"
          fill="#0078d4"
          font-size="11"
          font-weight="600"
          style="paint-order: stroke; stroke: rgba(255, 255, 255, 0.85); stroke-width: 3;"
        >
          R{{ t.row + 1 }}-C{{ t.col + 1 }}
        </text>
      </g>

      <!-- 拖拽中临时矩形 -->
      <rect
        v-if="dragRect"
        :x="dragRect.x"
        :y="dragRect.y"
        :width="dragRect.w"
        :height="dragRect.h"
        fill="rgba(16, 110, 190, 0.12)"
        stroke="#106ebe"
        stroke-width="2"
        stroke-dasharray="4 3"
        vector-effect="non-scaling-stroke"
      />
    </svg>

    <!-- 草稿确认/取消按钮浮层（贴在草稿右下角） -->
    <div
      v-if="pendingCrop && confirmButtonsPos"
      class="absolute flex items-center gap-2 pointer-events-none"
      :style="{
        left: confirmButtonsPos.left + 'px',
        top: confirmButtonsPos.top + 'px',
        transform: 'translate(-100%, 8px)',
      }"
    >
      <button
        class="pointer-events-auto btn-primary !py-1 !px-3 !text-xs"
        @click="emit('confirm-pending')"
      >
        {{ $t('crop.confirm') }}
      </button>
      <button
        class="pointer-events-auto btn-ghost !py-1 !px-3 !text-xs"
        @click="emit('cancel-pending')"
      >
        {{ $t('crop.cancel') }}
      </button>
    </div>
    <p
      v-if="pendingCrop"
      class="absolute left-3 bottom-3 text-xs text-gray-500 pointer-events-none"
    >
      {{ $t('crop.clickToCancelHint') }}
    </p>

    <!-- 省纸模式提示条：主图上不画 tile 网格，引导用户到"预览"视图按需试算 -->
    <div
      v-if="image && suppressTileGrid"
      class="absolute left-1/2 bottom-4 -translate-x-1/2 flex items-center gap-2 px-3 py-1.5
             rounded-fluent bg-amber-50 border border-amber-200 text-xs text-amber-700
             shadow-fluent1"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        class="w-4 h-4 shrink-0"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" />
      </svg>
      <span>省纸模式下无固定裁切示意</span>
      <button
        class="btn-primary !py-0.5 !px-2.5 !text-xs"
        @click="emit('request-preview')"
      >
        点击预览试算
      </button>
    </div>

    <!-- 放大镜 -->
    <div
      v-show="magnifierVisible && image"
      class="absolute top-4 right-4 rounded-fluent overflow-hidden ring-2 ring-white shadow-fluent2 pointer-events-none"
      :style="magnifierStyle"
    >
      <div class="absolute top-1/2 left-0 right-0 h-px bg-red-500/85"></div>
      <div class="absolute left-1/2 top-0 bottom-0 w-px bg-red-500/85"></div>
    </div>

    <!-- 未加载图片时的占位 -->
    <div
      v-if="!image"
      class="absolute inset-0 flex flex-col items-center justify-center text-gray-400 text-sm pointer-events-none"
    >
      <svg class="w-12 h-12 mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <circle cx="8.5" cy="8.5" r="1.5" fill="currentColor" />
        <path d="M21 15l-5-5L5 21" />
      </svg>
      <p>{{ $t('placeholders.noImage') }}</p>
    </div>
  </div>
</template>
