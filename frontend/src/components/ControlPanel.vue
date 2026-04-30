<script setup>
// ControlPanel 右侧控制面板。
// 通过 v-model:xxx 将所有可配置参数同步给父组件；不持有任何业务状态，便于热重载与单测。
import { computed } from 'vue';
import { PAPER_PRESETS } from '../lib/papers';

const props = defineProps({
  image: { type: Object, default: null },
  paperKey: { type: String, required: true },
  paperWmm: { type: Number, required: true },
  paperHmm: { type: Number, required: true },
  landscape: { type: Boolean, required: true },
  marginMm: { type: Number, required: true },
  overlapMm: { type: Number, required: true },
  targetWcm: { type: Number, required: true },
  targetHcm: { type: Number, required: true },
  lockAspect: { type: Boolean, required: true },
  exportMode: { type: String, required: true },
  allowRotate: { type: Boolean, required: true },
  warnings: { type: Array, default: () => [] },
  planCols: { type: Number, default: 0 },
  planRows: { type: Number, default: 0 },
  isExporting: { type: Boolean, default: false },
  baiduUpscale: { type: Boolean, default: false },
  upscaleHint: { type: String, default: '' },
  upscaleEligible: { type: Boolean, default: false },
  upscaleEnabled: { type: Boolean, default: false },
  baiduConfigured: { type: Boolean, default: false },
});

const emit = defineEmits([
  'update:paperKey',
  'update:paperWmm',
  'update:paperHmm',
  'update:landscape',
  'update:marginMm',
  'update:overlapMm',
  'update:targetWcm',
  'update:targetHcm',
  'update:lockAspect',
  'update:exportMode',
  'update:allowRotate',
  'open-image',
  'reset-crop',
  'export-pdf',
  'show-about',
  'update:upscaleEnabled',
  'open-baidu-config',
]);

// paperIsCustom 是否处在"自定义"分支，影响 W/H 输入框可编辑性。
const paperIsCustom = computed(() => props.paperKey === 'custom');

// 当选择预设时，自动写回 W/H；"自定义"则保留当前值。
function onSelectPreset(e) {
  const key = e.target.value;
  emit('update:paperKey', key);
  if (key !== 'custom') {
    const preset = PAPER_PRESETS.find((p) => p.key === key);
    if (preset) {
      emit('update:paperWmm', preset.widthMm);
      emit('update:paperHmm', preset.heightMm);
    }
  }
}

// DPI 文案描述：区分"缺失"、"低于 300 已提升"、"保留原值"三种来源。
const dpiDescription = computed(() => {
  const img = props.image;
  if (!img) return '';
  if (img.rawDpiX === 0) return `${img.dpiX} dpi（元数据缺失，按 300 处理）`;
  if (img.rawDpiX < 300) return `${img.rawDpiX} dpi → 已提升至 ${img.dpiX}`;
  return `${img.dpiX} dpi`;
});

// 省纸模式下横向开关被禁用，提示原因。
const landscapeDisabled = computed(() => props.exportMode === 'saving');
</script>

<template>
  <aside class="w-80 shrink-0 h-full overflow-y-auto bg-white border-l border-gray-200 flex flex-col">
    <header class="px-4 py-3 border-b border-gray-200 flex-shrink-0">
      <h1 class="text-base font-semibold text-gray-800">{{ $t('app.title') }}</h1>
      <p class="text-xs text-gray-500 mt-0.5">{{ $t('app.subtitle') }}</p>
    </header>

    <!-- 文件操作 -->
    <section class="p-4 border-b border-gray-200 flex flex-col gap-2">
      <button class="btn-primary" @click="emit('open-image')">
        {{ $t('actions.openImage') }}
      </button>
      <button class="btn-ghost" :disabled="!image" @click="emit('reset-crop')">
        {{ $t('actions.resetCrop') }}
      </button>
    </section>

    <!-- 图片信息 -->
    <section v-if="image" class="p-4 border-b border-gray-200">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
        {{ $t('panels.imageInfo') }}
      </h2>
      <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt class="text-gray-500">{{ $t('panels.size') }}</dt>
        <dd class="text-gray-800">{{ image.width }} × {{ image.height }} px</dd>
        <dt class="text-gray-500">{{ $t('panels.format') }}</dt>
        <dd class="text-gray-800 uppercase">{{ image.format }}</dd>
        <dt class="text-gray-500">{{ $t('panels.dpi') }}</dt>
        <dd class="text-gray-800">{{ dpiDescription }}</dd>
      </dl>
    </section>

    <!-- 纸张 -->
    <section class="p-4 border-b border-gray-200 space-y-3">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide">
        {{ $t('panels.paper') }}
      </h2>
      <div>
        <label class="label-base">{{ $t('panels.paperPreset') }}</label>
        <select class="input-base" :value="paperKey" @change="onSelectPreset">
          <option v-for="p in PAPER_PRESETS" :key="p.key" :value="p.key">
            {{ $t(p.labelKey) }}
          </option>
        </select>
      </div>
      <div class="grid grid-cols-2 gap-2">
        <div>
          <label class="label-base">{{ $t('panels.paperW') }}</label>
          <input
            type="number"
            class="input-base"
            :disabled="!paperIsCustom"
            :value="paperWmm"
            min="10"
            step="1"
            @input="emit('update:paperWmm', Number($event.target.value))"
          />
        </div>
        <div>
          <label class="label-base">{{ $t('panels.paperH') }}</label>
          <input
            type="number"
            class="input-base"
            :disabled="!paperIsCustom"
            :value="paperHmm"
            min="10"
            step="1"
            @input="emit('update:paperHmm', Number($event.target.value))"
          />
        </div>
      </div>
      <label
        class="flex items-center gap-2 text-sm select-none"
        :class="landscapeDisabled && 'opacity-60 cursor-not-allowed'"
        :title="landscapeDisabled ? $t('panels.landscapeDisabledTip') : ''"
      >
        <input
          type="checkbox"
          class="w-4 h-4 accent-fluent-500"
          :checked="landscape"
          :disabled="landscapeDisabled"
          @change="emit('update:landscape', $event.target.checked)"
        />
        <span>{{ $t('panels.landscape') }}</span>
      </label>
    </section>

    <!-- 打印参数 -->
    <section class="p-4 border-b border-gray-200 space-y-3">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide">
        {{ $t('panels.printing') }}
      </h2>
      <div class="grid grid-cols-2 gap-2">
        <div>
          <label class="label-base">{{ $t('panels.margin') }}</label>
          <input
            type="number"
            class="input-base"
            :value="marginMm"
            min="0"
            step="0.5"
            @input="emit('update:marginMm', Number($event.target.value))"
          />
        </div>
        <div>
          <label class="label-base">{{ $t('panels.overlap') }}</label>
          <input
            type="number"
            class="input-base"
            :value="overlapMm"
            min="0"
            step="0.5"
            @input="emit('update:overlapMm', Number($event.target.value))"
          />
        </div>
      </div>
    </section>

    <!-- 目标尺寸 -->
    <section class="p-4 border-b border-gray-200 space-y-3">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide">
        {{ $t('panels.target') }}
      </h2>
      <div class="grid grid-cols-2 gap-2">
        <div>
          <label class="label-base">{{ $t('panels.targetW') }}</label>
          <input
            type="number"
            class="input-base"
            :value="targetWcm"
            min="0"
            step="0.1"
            @input="emit('update:targetWcm', Number($event.target.value))"
          />
        </div>
        <div>
          <label class="label-base">{{ $t('panels.targetH') }}</label>
          <input
            type="number"
            class="input-base"
            :value="targetHcm"
            min="0"
            step="0.1"
            @input="emit('update:targetHcm', Number($event.target.value))"
          />
        </div>
      </div>
      <label class="flex items-center gap-2 text-sm select-none">
        <input
          type="checkbox"
          class="w-4 h-4 accent-fluent-500"
          :checked="lockAspect"
          @change="emit('update:lockAspect', $event.target.checked)"
        />
        <span>{{ $t('panels.lockAspect') }}</span>
      </label>
    </section>

    <!-- 百度配置入口 -->
    <section class="p-4 border-b border-gray-200 space-y-2">
      <button class="btn-ghost w-full" type="button" @click="emit('open-baidu-config')">
        配置百度云
      </button>
      <p
        v-if="!baiduConfigured"
        class="text-xs text-amber-700 leading-relaxed rounded-fluent border border-amber-200 bg-amber-50/80 px-3 py-2"
      >
        <span class="font-medium">AI 放大未启用：</span>{{ upscaleHint || '尚未配置 AK/SK' }}
      </p>
    </section>

    <!-- 百度清晰度增强（可选，需 secrets + 满足接口边长/比例限制） -->
    <section v-if="baiduUpscale" class="p-4 border-b border-gray-200 space-y-2">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide">
        {{ $t('panels.upscaleTitle') }}
      </h2>
      <p class="text-xs text-gray-500 leading-relaxed">
        {{ $t('panels.upscaleHint') }}
      </p>
      <div
        class="rounded-fluent border border-gray-200 bg-gray-50/80 px-3 py-2 text-xs text-gray-700"
      >
        {{ $t('panels.upscaleFixedParams') }}
      </div>
      <label
        class="flex items-center gap-2 text-sm select-none"
        :class="(!upscaleEligible || !image) && 'opacity-60'"
      >
        <input
          type="checkbox"
          class="w-4 h-4 accent-fluent-500"
          :checked="upscaleEnabled"
          :disabled="!image || !upscaleEligible || isExporting"
          @change="emit('update:upscaleEnabled', $event.target.checked)"
        />
        <span>导出前使用百度云端增强</span>
      </label>
      <p
        v-if="image && !upscaleEligible"
        class="text-xs text-amber-600 leading-relaxed"
      >
        {{ $t('panels.upscaleUnavailable') }}
      </p>
    </section>

    <!-- 导出模式 -->
    <section class="p-4 border-b border-gray-200 space-y-2">
      <h2 class="text-xs font-semibold text-gray-500 uppercase tracking-wide">
        {{ $t('panels.exportMode') }}
      </h2>
      <div class="flex gap-2">
        <label
          class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-fluent border text-sm cursor-pointer transition-colors"
          :class="
            exportMode === 'standard'
              ? 'bg-fluent-50 border-fluent-500 text-fluent-700'
              : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
          "
        >
          <input
            type="radio"
            class="hidden"
            value="standard"
            :checked="exportMode === 'standard'"
            @change="emit('update:exportMode', 'standard')"
          />
          {{ $t('panels.modeStandard') }}
        </label>
        <label
          class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-fluent border text-sm cursor-pointer transition-colors"
          :class="
            exportMode === 'saving'
              ? 'bg-fluent-50 border-fluent-500 text-fluent-700'
              : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
          "
        >
          <input
            type="radio"
            class="hidden"
            value="saving"
            :checked="exportMode === 'saving'"
            @change="emit('update:exportMode', 'saving')"
          />
          {{ $t('panels.modePaperSaving') }}
        </label>
      </div>
      <label
        v-if="exportMode === 'saving'"
        class="flex items-center gap-2 text-sm select-none"
      >
        <input
          type="checkbox"
          class="w-4 h-4 accent-fluent-500"
          :checked="allowRotate"
          @change="emit('update:allowRotate', $event.target.checked)"
        />
        <span>{{ $t('panels.allowRotate') }}</span>
      </label>
      <p v-if="exportMode === 'saving'" class="text-xs text-amber-600 leading-relaxed">
        {{ $t('panels.saverHint') }}
      </p>
    </section>

    <!-- 切分统计 + 告警 -->
    <section v-if="image" class="p-4 border-b border-gray-200 space-y-2">
      <div class="text-xs text-gray-500">
        Cols × Rows:
        <span class="text-gray-800 font-medium ml-1">{{ planCols }} × {{ planRows }}</span>
        <span class="text-gray-400 ml-1">（共 {{ planCols * planRows }} 张）</span>
      </div>
      <p
        v-for="(w, i) in warnings"
        :key="i"
        class="text-xs text-amber-600 leading-relaxed"
      >{{ w }}</p>
    </section>

    <!-- 操作栏 -->
    <section class="mt-auto p-4 flex flex-col gap-2">
      <button
        class="btn-primary"
        :disabled="!image || isExporting"
        @click="emit('export-pdf')"
      >
        {{ isExporting ? $t('export.dialogTitle') : $t('actions.exportPdf') }}
      </button>
      <button
        class="text-xs text-gray-400 hover:text-fluent-600 transition-colors self-center"
        type="button"
        @click="emit('show-about')"
      >
        关于本软件
      </button>
    </section>
  </aside>
</template>
