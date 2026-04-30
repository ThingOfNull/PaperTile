<script setup>
import { ref, watch } from 'vue';

const props = defineProps({
  visible: { type: Boolean, default: false },
  initialApiKey: { type: String, default: '' },
  initialSecretKey: { type: String, default: '' },
  configPath: { type: String, default: '' },
  saving: { type: Boolean, default: false },
});

const emit = defineEmits(['close', 'save']);

const apiKey = ref('');
const secretKey = ref('');

watch(
  () => props.visible,
  (show) => {
    if (!show) return;
    apiKey.value = props.initialApiKey ?? '';
    secretKey.value = props.initialSecretKey ?? '';
  },
);

function onSave() {
  emit('save', {
    apiKey: apiKey.value.trim(),
    secretKey: secretKey.value.trim(),
  });
}
</script>

<template>
  <div
    v-if="visible"
    class="fixed inset-0 z-[70] flex items-center justify-center bg-black/35 px-4"
    @click.self="emit('close')"
  >
    <div class="w-full max-w-lg rounded-fluent bg-white shadow-xl border border-gray-200 p-5">
      <h3 class="text-base font-semibold text-gray-800">配置百度云</h3>
      <p class="text-xs text-gray-500 mt-1">
        仅保存在本机：<code class="text-[11px]">{{ configPath || 'config/config.json' }}</code>
      </p>

      <div class="mt-4 space-y-3">
        <div>
          <label class="label-base">API Key (AK)</label>
          <input
            v-model="apiKey"
            class="input-base"
            type="text"
            autocomplete="off"
            placeholder="请输入百度云 API Key"
          />
        </div>
        <div>
          <label class="label-base">Secret Key (SK)</label>
          <input
            v-model="secretKey"
            class="input-base"
            type="password"
            autocomplete="off"
            placeholder="请输入百度云 Secret Key"
          />
        </div>
      </div>

      <div class="mt-5 flex items-center justify-end gap-2">
        <button class="btn-ghost" :disabled="saving" @click="emit('close')">取消</button>
        <button
          class="btn-primary"
          :disabled="saving || !apiKey.trim() || !secretKey.trim()"
          @click="onSave"
        >
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>
    </div>
  </div>
</template>
