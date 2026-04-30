<script setup>
// AboutDialog 关于对话框：展示产品名、版本、仓库地址、开源协议。
// 纯展示组件，外部用 v-model:visible / @close 控制显隐。
defineProps({
  visible: { type: Boolean, required: true },
});
const emit = defineEmits(['close']);

const REPO_URL = 'https://github.com/ThingOfNull/PaperTile';
const VERSION = '1.0.0';

// Wails 环境下 <a target="_blank"> 会被 webview 拦成白屏，
// 所以用 runtime.BrowserOpenURL 调起系统默认浏览器。
function openExternal(url) {
  // 懒加载：Wails 注入的 runtime 在浏览器静态 preview 里是不存在的。
  const bo = window?.runtime?.BrowserOpenURL;
  if (typeof bo === 'function') {
    bo(url);
  } else {
    window.open(url, '_blank', 'noopener');
  }
}
</script>

<template>
  <Transition name="fade">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      @click.self="emit('close')"
    >
      <div
        class="w-[380px] rounded-fluent bg-white shadow-fluent2 border border-gray-200
               p-6 flex flex-col items-center text-center"
      >
        <div class="text-5xl mb-3 select-none" aria-hidden="true">✂️</div>
        <h2 class="text-lg font-semibold text-gray-800">打印切图拼版工具</h2>
        <p class="text-xs text-gray-500 mt-1">Version {{ VERSION }}</p>

        <p class="text-sm text-gray-600 leading-relaxed mt-4">
          将一张图片按指定物理尺寸拆成多页 A4 / A3 / B4 打印，<br />
          支持自动裁切、重叠补偿与省纸拼版。
        </p>

        <div class="w-full mt-5 pt-4 border-t border-gray-100 space-y-2 text-sm">
          <div class="flex items-center justify-center gap-2 text-gray-600">
            <span class="text-gray-400">仓库</span>
            <a
              href="#"
              class="text-fluent-600 hover:underline break-all"
              @click.prevent="openExternal(REPO_URL)"
            >{{ REPO_URL }}</a>
          </div>
          <div class="text-xs text-gray-500">
            基于 <span class="font-medium text-gray-700">MIT License</span> 开源 · 欢迎 Issue / PR
          </div>
        </div>

        <button class="btn-primary mt-6 w-24" @click="emit('close')">关闭</button>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
