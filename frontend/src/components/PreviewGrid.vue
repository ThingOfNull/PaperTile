<script setup>
// PreviewGrid 拼版预览 + 拣选视图，支持两种布局：
//
// 1) 标准模式（plan.packedPages 为空）：
//    把 plan.tiles 按二维 (row, col) 绝对定位排列，块间留一点视觉间隙，还原打印时的相对位置。
//    每个 tile 左上角放 Checkbox；未勾选的 tile 置灰 + 半透明，导出时整页跳过。
//
// 2) 省纸模式（plan.packedPages 非空）：
//    按"页"渲染 —— 每张 PDF 页一张"纸"，tile 按 packer 计算出的毫米坐标绝对定位，
//    必要时旋转 90°。同样支持点击勾选/跳过（跳过的 tile 会触发 BuildPlan 重算）。
//
// 坐标系说明：
//   - plan.sourceW/H、tile.x0..y1 均为"缩放后源图"像素（target 像素）
//   - image.previewDataUrl 是对"原图"按 previewLongSide 缩放得到的缩略底图
//   - activeRectInPreview 是"被切分区域"（= 裁剪框）在 preview 画布上的矩形
//   - 标准模式：target 像素 → preview 像素用 active.w / sourceW
//   - 省纸模式：mm → CSS 用 displayScaleMm（CSS px / mm），target 像素 → CSS 复用 preview 的 fx + k
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

const props = defineProps({
  image: { type: Object, default: null },    // ImageInfo
  plan: { type: Object, default: null },     // PlanResponse
  appliedCrop: { type: Object, default: null }, // 已确认裁剪（原图像素）
  skippedTiles: { type: Set, default: () => new Set() }, // Set<'row,col'>
});

const emit = defineEmits(['toggle-tile', 'clear-skipped']);

// 块间视觉间隙（CSS px）。纯美术决定，与物理重叠/边距无关。
const GAP_PX = 10;
// 可视区外侧留白（CSS px）。两侧一共占 2× 该值。
// 原先 24 导致标准模式网格被挤得偏小，这里压到 12 给网格让出更多显示空间。
const CANVAS_PAD_PX = 12;
// 省纸模式下单张"纸"的最大 CSS 宽度。太宽反而看不清楚整体布局。
const PACKED_PAGE_MAX_W = 640;
// 省纸模式下 CSS/mm 的硬顶，避免在超大显示器上把 A4 纸放大到看不见邻居。
// 3 CSS/mm 约等于 A4 portrait = 630 × 891 px，观感刚刚好。
const PACKED_SCALE_CAP = 3;
// 省纸模式下页标签 + 上下间距预留（CSS px），让"一页恰好放进视口"的高度约束落地。
const PACKED_PAGE_LABEL_H = 48;
// 省纸模式下页与页之间的纵向间隔（CSS px）。
const PACKED_PAGE_GAP = 24;
// 省纸模式下"已跳过"抽屉里每个缩略图的长边尺寸（CSS px）。
const SKIPPED_THUMB_LONG_EDGE = 110;

// ===== 响应式尺寸 =====
// 跟随窗口变化，和 ImageCanvas 一样。contentBoxSize 以 bodyRef 的 clientWidth/Height 为准。
const bodyRef = ref(null);
const contentSize = ref({ w: 0, h: 0 });
let resizeObserver = null;
onMounted(() => {
  if (!bodyRef.value) return;
  const apply = () => {
    if (!bodyRef.value) return;
    contentSize.value = {
      w: bodyRef.value.clientWidth,
      h: bodyRef.value.clientHeight,
    };
  };
  apply();
  resizeObserver = new ResizeObserver(apply);
  resizeObserver.observe(bodyRef.value);
});
onBeforeUnmount(() => {
  if (resizeObserver) resizeObserver.disconnect();
});

// 被切分区域在 preview 画布上的矩形：裁剪优先，没有则整张 preview。
const activeRectInPreview = computed(() => {
  const img = props.image;
  if (!img) return null;
  const crop = props.appliedCrop;
  if (!crop) {
    return { x: 0, y: 0, w: img.previewWidth, h: img.previewHeight };
  }
  const sx = img.previewWidth / img.width;
  const sy = img.previewHeight / img.height;
  return {
    x: crop.x0 * sx,
    y: crop.y0 * sy,
    w: (crop.x1 - crop.x0) * sx,
    h: (crop.y1 - crop.y0) * sy,
  };
});

// ===== 模式判定 =====
// 省纸模式 = 后端返回了 packedPages；否则走标准网格视图。
const isPacked = computed(() => Array.isArray(props.plan?.packedPages) && props.plan.packedPages.length > 0);

// =================== 标准模式计算 ===================
// displayScale：CSS px / target px。让 cols × rows 的整张网格自适应容器。
const displayScale = computed(() => {
  const p = props.plan;
  if (!p || !p.cols || !p.rows || !p.tilePxW || !p.tilePxH) return 1;
  const avW = Math.max(0, contentSize.value.w - CANVAS_PAD_PX * 2);
  const avH = Math.max(0, contentSize.value.h - CANVAS_PAD_PX * 2);
  if (avW <= 0 || avH <= 0) return 0.1;
  const scaleX = (avW - (p.cols - 1) * GAP_PX) / (p.cols * p.tilePxW);
  const scaleY = (avH - (p.rows - 1) * GAP_PX) / (p.rows * p.tilePxH);
  const s = Math.min(scaleX, scaleY);
  return s > 0 ? s : 0.02;
});

const tileViews = computed(() => {
  const p = props.plan;
  const img = props.image;
  const active = activeRectInPreview.value;
  if (!p || !img || !active) return [];
  const ds = displayScale.value;
  const fx = active.w / p.sourceW;
  const fy = active.h / p.sourceH;
  return (p.tiles || []).map((t) => {
    const tw = t.x1 - t.x0;
    const th = t.y1 - t.y0;
    const cellW = tw * ds;
    const cellH = th * ds;
    const k = cellW / (tw * fx);
    return {
      key: `${t.row},${t.col}`,
      col: t.col,
      row: t.row,
      left: t.col * (p.tilePxW * ds + GAP_PX),
      top: t.row * (p.tilePxH * ds + GAP_PX),
      width: cellW,
      height: cellH,
      bgWidth: img.previewWidth * k,
      bgHeight: img.previewHeight * k,
      bgPosX: -(active.x + t.x0 * fx) * k,
      bgPosY: -(active.y + t.y0 * fy) * k,
    };
  });
});

const gridSize = computed(() => {
  const p = props.plan;
  if (!p || !p.cols || !p.rows) return { w: 0, h: 0 };
  const ds = displayScale.value;
  return {
    w: p.cols * (p.tilePxW * ds + GAP_PX) - GAP_PX,
    h: p.rows * (p.tilePxH * ds + GAP_PX) - GAP_PX,
  };
});

// =================== 省纸模式计算 ===================
// 目标：让"最苛刻的那一页"同时满足：
//   1) 横向塞得下（容器宽度 - 左右 padding，且不超过 PACKED_PAGE_MAX_W 的美术上限）；
//   2) 纵向塞得下（容器高度 - 页标签预留），保证至少一页完整可见，而不是一点开就要滚半天；
//   3) CSS/mm 不超过 PACKED_SCALE_CAP，避免 4K 大屏上把 A4 撑到荒诞尺寸。
// 跨页取最严约束，这样所有页方向/尺寸混合时也不会有哪一页溢出。
const packedScale = computed(() => {
  if (!isPacked.value) return 1;
  const pages = props.plan.packedPages;
  if (!pages.length) return 1;
  const maxUsableW = pages.reduce((m, pg) => Math.max(m, pg.usableWMm), 0);
  const maxUsableH = pages.reduce((m, pg) => Math.max(m, pg.usableHMm), 0);
  if (maxUsableW <= 0 || maxUsableH <= 0) return 1;
  const avW = Math.max(0, contentSize.value.w - CANVAS_PAD_PX * 2);
  const avH = Math.max(0, contentSize.value.h - CANVAS_PAD_PX * 2 - PACKED_PAGE_LABEL_H);
  if (avW <= 0 || avH <= 0) return 0.5;
  const byWidth = Math.min(avW, PACKED_PAGE_MAX_W) / maxUsableW;
  const byHeight = avH / maxUsableH;
  const s = Math.min(byWidth, byHeight, PACKED_SCALE_CAP);
  return s > 0 ? s : 0.5;
});

// 把后端每张 PackedPage 展开成渲染所需的所有几何数据（已转 CSS px）。
// 对 rotated tile：用 inner div + transform: rotate(-90deg) 做视觉旋转，
// 和后端 imaging.Rotate90（CCW）保持方向一致。
const packedViews = computed(() => {
  const plan = props.plan;
  const img = props.image;
  const active = activeRectInPreview.value;
  if (!isPacked.value || !plan || !img || !active) return [];
  const s = packedScale.value;
  const fx = active.w / plan.sourceW;
  const fy = active.h / plan.sourceH;
  return plan.packedPages.map((pg) => {
    const pageW = pg.usableWMm * s;
    const pageH = pg.usableHMm * s;
    const tiles = pg.tiles.map((t) => {
      const pxW = t.x1 - t.x0;
      const pxH = t.y1 - t.y0;
      // 放置框（outer）尺寸 = 装箱给出的 place 尺寸。
      const slotW = t.placeWMm * s;
      const slotH = t.placeHMm * s;
      // 原始（未旋转）tile 在 CSS 中的尺寸：rotated 时宽高互换。
      const natW = t.rotated ? slotH : slotW;
      const natH = t.rotated ? slotW : slotH;
      // k：preview px → CSS px 的比例。rotated 与否都用 natW（未旋转宽）对齐原始宽。
      const k = natW / (pxW * fx);
      return {
        key: `${t.row},${t.col}`,
        col: t.col,
        row: t.row,
        rotated: t.rotated,
        left: t.xMm * s,
        top: t.yMm * s,
        slotW,
        slotH,
        natW,
        natH,
        bgWidth: img.previewWidth * k,
        bgHeight: img.previewHeight * k,
        bgPosX: -(active.x + t.x0 * fx) * k,
        bgPosY: -(active.y + t.y0 * fy) * k,
      };
    });
    return {
      index: pg.index,
      landscape: pg.landscape,
      pageW,
      pageH,
      tiles,
    };
  });
});

function isSkipped(key) {
  return props.skippedTiles.has(key);
}

function onToggle(col, row) {
  emit('toggle-tile', { col, row });
}

const skippedCount = computed(() => props.skippedTiles.size);
const totalTiles = computed(() => props.plan?.tiles?.length ?? 0);
const packedPageCount = computed(() => props.plan?.packedPages?.length ?? 0);

// 省纸模式专用的"已跳过"抽屉数据。
// 为什么需要这玩意：省纸模式下后端会把跳过的 tile 从装箱里剔除，所以它们不会出现在任何一页的画面里；
// 没有这个抽屉用户就没有"再勾回来"的入口，只能点顶部"全选"一把全撤。
// 缩略图沿用主预览图 + CSS 背景定位，零额外 IPC。
const skippedTrayViews = computed(() => {
  const plan = props.plan;
  const img = props.image;
  const active = activeRectInPreview.value;
  if (!isPacked.value || !plan || !img || !active) return [];
  if (props.skippedTiles.size === 0) return [];
  const fx = active.w / plan.sourceW;
  const fy = active.h / plan.sourceH;
  return (plan.tiles || [])
    .filter((t) => isSkipped(`${t.row},${t.col}`))
    .map((t) => {
      const pxW = t.x1 - t.x0;
      const pxH = t.y1 - t.y0;
      // 让每个缩略图的长边都等于 SKIPPED_THUMB_LONG_EDGE，保持宽高比。
      const s = SKIPPED_THUMB_LONG_EDGE / Math.max(pxW, pxH);
      const cssW = pxW * s;
      const cssH = pxH * s;
      // k：preview px → 缩略图 CSS px。
      const k = cssW / (pxW * fx);
      return {
        key: `${t.row},${t.col}`,
        col: t.col,
        row: t.row,
        width: cssW,
        height: cssH,
        bgWidth: img.previewWidth * k,
        bgHeight: img.previewHeight * k,
        bgPosX: -(active.x + t.x0 * fx) * k,
        bgPosY: -(active.y + t.y0 * fy) * k,
      };
    });
});
</script>

<template>
  <div class="relative w-full h-full flex flex-col bg-slate-100 overflow-hidden">
    <!-- 顶部信息条 -->
    <div
      v-if="plan && totalTiles > 0"
      class="shrink-0 bg-white/90 backdrop-blur px-4 py-2 border-b border-gray-200
             flex items-center gap-4 text-sm"
    >
      <span class="text-gray-700">
        共 <b class="tabular-nums">{{ totalTiles }}</b> 块，
        <span v-if="skippedCount > 0" class="text-amber-600">
          已跳过 <b class="tabular-nums">{{ skippedCount }}</b> 块
        </span>
        <span v-else class="text-gray-500">全部将输出</span>
        <span v-if="isPacked" class="text-gray-500">
          · 装箱后 <b class="tabular-nums">{{ packedPageCount }}</b> 页
        </span>
      </span>
      <button
        v-if="skippedCount > 0"
        class="btn-ghost text-xs px-2 py-1"
        @click="emit('clear-skipped')"
      >
        全选
      </button>
      <span class="ml-auto text-xs text-gray-500">
        取消勾选的图块在标准模式下整页不输出，省纸模式下不参与打包。
      </span>
    </div>

    <!-- 可视区：随容器尺寸变化；ResizeObserver 挂在这里 -->
    <div
      ref="bodyRef"
      class="relative flex-1 overflow-auto"
      :class="isPacked ? 'overflow-y-auto' : 'overflow-hidden'"
    >
      <!-- 空状态 -->
      <div
        v-if="!plan || totalTiles === 0"
        class="absolute inset-0 flex items-center justify-center text-gray-400"
      >
        暂无可预览的切分结果
      </div>

      <!-- ===== 标准模式：二维网格 ===== -->
      <div
        v-else-if="!isPacked"
        class="absolute"
        :style="{
          left: '50%',
          top: '50%',
          width: gridSize.w + 'px',
          height: gridSize.h + 'px',
          transform: 'translate(-50%, -50%)',
        }"
      >
        <div
          v-for="t in tileViews"
          :key="t.key"
          class="absolute group select-none rounded-sm overflow-hidden ring-1 transition-all"
          :class="isSkipped(t.key)
            ? 'ring-gray-300'
            : 'ring-fluent-500/40 hover:ring-fluent-500'"
          :style="{
            left: t.left + 'px',
            top: t.top + 'px',
            width: t.width + 'px',
            height: t.height + 'px',
          }"
          @click="onToggle(t.col, t.row)"
        >
          <div
            class="absolute inset-0 bg-no-repeat"
            :class="isSkipped(t.key) ? 'opacity-25 grayscale' : ''"
            :style="{
              backgroundImage: image ? `url(${image.previewDataUrl})` : 'none',
              backgroundSize: `${t.bgWidth}px ${t.bgHeight}px`,
              backgroundPosition: `${t.bgPosX}px ${t.bgPosY}px`,
            }"
          ></div>

          <div
            v-if="isSkipped(t.key)"
            class="absolute inset-0 pointer-events-none"
            style="background-image: repeating-linear-gradient(
              45deg,
              rgba(148, 163, 184, 0.25) 0 4px,
              transparent 4px 10px);"
          ></div>

          <div
            class="absolute top-1 left-1 w-5 h-5 rounded-sm border-2 flex items-center justify-center
                   bg-white/90 shadow-sm transition-colors"
            :class="isSkipped(t.key)
              ? 'border-gray-400 text-gray-300'
              : 'border-fluent-500 text-fluent-500'"
          >
            <svg
              v-if="!isSkipped(t.key)"
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="3.5"
              class="w-3.5 h-3.5"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 12l5 5L20 7" />
            </svg>
          </div>

          <div
            class="absolute bottom-1 right-1 px-1.5 py-0.5 text-[10px] font-mono rounded
                   bg-black/55 text-white tabular-nums pointer-events-none"
          >
            R{{ t.row + 1 }}-C{{ t.col + 1 }}
          </div>
        </div>
      </div>

      <!-- ===== 省纸模式：按页流式展示 ===== -->
      <div
        v-else
        class="flex flex-col items-center"
        :style="{ padding: CANVAS_PAD_PX + 'px', gap: PACKED_PAGE_GAP + 'px' }"
      >
        <div
          v-for="pg in packedViews"
          :key="pg.index"
          class="flex flex-col items-center"
        >
          <div class="text-xs text-gray-500 mb-1.5 tabular-nums">
            第 {{ pg.index + 1 }} / {{ packedPageCount }} 页 · {{ pg.landscape ? '横向' : '纵向' }}
          </div>
          <div
            class="relative bg-white shadow-fluent1 ring-1 ring-gray-300 rounded-sm"
            :style="{ width: pg.pageW + 'px', height: pg.pageH + 'px' }"
          >
            <!-- 每个 tile：outer 是 packer 给出的放置区域；inner 是原始未旋转 tile，必要时旋转 -->
            <div
              v-for="t in pg.tiles"
              :key="t.key"
              class="absolute overflow-hidden ring-1 cursor-pointer transition-all"
              :class="isSkipped(t.key)
                ? 'ring-gray-300'
                : 'ring-fluent-500/40 hover:ring-fluent-500'"
              :style="{
                left: t.left + 'px',
                top: t.top + 'px',
                width: t.slotW + 'px',
                height: t.slotH + 'px',
              }"
              @click="onToggle(t.col, t.row)"
            >
              <!-- 旋转容器：rotated 时绕中心逆时针 90°，对应后端 imaging.Rotate90 -->
              <div
                class="absolute left-1/2 top-1/2"
                :style="{
                  width: t.natW + 'px',
                  height: t.natH + 'px',
                  transform: t.rotated
                    ? 'translate(-50%, -50%) rotate(-90deg)'
                    : 'translate(-50%, -50%)',
                  transformOrigin: 'center center',
                }"
              >
                <div
                  class="absolute inset-0 bg-no-repeat"
                  :class="isSkipped(t.key) ? 'opacity-25 grayscale' : ''"
                  :style="{
                    backgroundImage: image ? `url(${image.previewDataUrl})` : 'none',
                    backgroundSize: `${t.bgWidth}px ${t.bgHeight}px`,
                    backgroundPosition: `${t.bgPosX}px ${t.bgPosY}px`,
                  }"
                ></div>
              </div>

              <div
                v-if="isSkipped(t.key)"
                class="absolute inset-0 pointer-events-none"
                style="background-image: repeating-linear-gradient(
                  45deg,
                  rgba(148, 163, 184, 0.25) 0 4px,
                  transparent 4px 10px);"
              ></div>

              <!-- Checkbox（始终随 outer，不随 inner 旋转） -->
              <div
                class="absolute top-1 left-1 w-5 h-5 rounded-sm border-2 flex items-center justify-center
                       bg-white/90 shadow-sm transition-colors"
                :class="isSkipped(t.key)
                  ? 'border-gray-400 text-gray-300'
                  : 'border-fluent-500 text-fluent-500'"
              >
                <svg
                  v-if="!isSkipped(t.key)"
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="3.5"
                  class="w-3.5 h-3.5"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 12l5 5L20 7" />
                </svg>
              </div>

              <!-- 坐标水印：带 R 标表示后端会旋转 -->
              <div
                class="absolute bottom-1 right-1 px-1.5 py-0.5 text-[10px] font-mono rounded
                       bg-black/55 text-white tabular-nums pointer-events-none"
              >
                R{{ t.row + 1 }}-C{{ t.col + 1 }}<span v-if="t.rotated" class="ml-0.5">↻</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 已跳过抽屉：省纸模式专用。跳过的 tile 不进装箱不占页面，但这里提供一个"再勾回来"的入口 -->
        <div
          v-if="skippedTrayViews.length > 0"
          class="w-full max-w-3xl mt-2 p-3 rounded-fluent bg-white/70 border border-gray-200"
        >
          <div class="flex items-center gap-2 mb-2 text-xs text-gray-600">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              class="w-4 h-4"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
            <span>
              已跳过 <b class="tabular-nums">{{ skippedTrayViews.length }}</b> 块 ·
              点击缩略图可重新加入打包
            </span>
          </div>
          <div class="flex flex-wrap gap-2">
            <div
              v-for="t in skippedTrayViews"
              :key="t.key"
              class="relative rounded overflow-hidden ring-1 ring-gray-300 hover:ring-fluent-500
                     cursor-pointer transition-all"
              :style="{ width: t.width + 'px', height: t.height + 'px' }"
              title="点击重新加入打包"
              @click="onToggle(t.col, t.row)"
            >
              <div
                class="absolute inset-0 bg-no-repeat opacity-60 grayscale"
                :style="{
                  backgroundImage: image ? `url(${image.previewDataUrl})` : 'none',
                  backgroundSize: `${t.bgWidth}px ${t.bgHeight}px`,
                  backgroundPosition: `${t.bgPosX}px ${t.bgPosY}px`,
                }"
              ></div>
              <div
                class="absolute inset-0 pointer-events-none"
                style="background-image: repeating-linear-gradient(
                  45deg,
                  rgba(148, 163, 184, 0.3) 0 4px,
                  transparent 4px 10px);"
              ></div>
              <!-- 左上空心 checkbox，鼠标移上去变成勾 -->
              <div
                class="absolute top-1 left-1 w-5 h-5 rounded-sm border-2 border-gray-400
                       bg-white/90 shadow-sm flex items-center justify-center
                       text-transparent group-hover:text-fluent-500"
              ></div>
              <div
                class="absolute bottom-0.5 right-1 text-[10px] font-mono rounded
                       bg-black/55 text-white px-1 tabular-nums pointer-events-none"
              >
                R{{ t.row + 1 }}-C{{ t.col + 1 }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
