<script setup>
// ExportProgressOverlay 导出中遮罩。
// 三个职责：
//   1. 在全屏半透明蒙层上渲染一个卡片；
//   2. 根据当前 stage + percent 显示进度条与阶段文案；
//   3. visible=false 时渲染为占位 null（由 v-if 控制，可平滑关闭）。
// 目前没有"取消"按钮；取消能力在 Step 6 引入。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  visible: { type: Boolean, default: false },
  stage: { type: String, default: '' },
  percent: { type: Number, default: 0 },
  detail: { type: String, default: '' },
});

const { t } = useI18n();

const STAGE_KEY_MAP = {
  cropping: 'export.stageCropping',
  uploading: 'export.stageUploading',
  upscaling: 'export.stageUpscaling',
  downloading: 'export.stageDownloading',
  scaling: 'export.stageScaling',
  tiling: 'export.stageTiling',
  encoding: 'export.stageEncoding',
  writing: 'export.stageWriting',
  completed: 'export.stageCompleted',
};

const stageLabel = computed(() => {
  const key = STAGE_KEY_MAP[props.stage];
  return key ? t(key) : '';
});
const clampedPercent = computed(() =>
  Math.max(0, Math.min(100, Math.round(props.percent || 0))),
);
</script>

<template>
  <transition name="fade">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-sm"
    >
      <div class="card w-[380px] p-5">
        <h3 class="text-base font-semibold text-gray-800 mb-3">
          {{ $t('export.dialogTitle') }}
        </h3>
        <p
          class="text-sm text-gray-600 mb-1"
          :class="!detail && 'mb-3'"
        >{{ stageLabel || stage || '…' }}</p>
        <p
          v-if="detail"
          class="text-xs text-gray-500 mb-3 leading-relaxed"
        >{{ detail }}</p>
        <div class="h-2 w-full rounded-full bg-gray-100 overflow-hidden">
          <div
            class="h-full bg-fluent-500 transition-all duration-150"
            :style="{ width: clampedPercent + '%' }"
          ></div>
        </div>
        <p class="mt-2 text-xs text-gray-500 tabular-nums">
          {{ clampedPercent }}%
        </p>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
