import { ref, watch } from 'vue';
import { buildPlan } from '../lib/backend';

// usePlan 监听切分参数并自动调用后端 BuildPlan。
// 行为：
//   - 参数变更后按 debounceMs 节流；
//   - 成功写入 plan，失败写入 error；
//   - 若 paramsRef 返回 null 视为"条件不完整"，跳过请求。
export function usePlan(paramsRef, { debounceMs = 120 } = {}) {
  const plan = ref(null);
  const error = ref(null);
  const loading = ref(false);
  let timer = null;

  watch(
    paramsRef,
    (p) => {
      if (!p) {
        plan.value = null;
        error.value = null;
        return;
      }
      if (timer) clearTimeout(timer);
      timer = setTimeout(async () => {
        loading.value = true;
        try {
          const result = await buildPlan(p);
          plan.value = result;
          error.value = null;
        } catch (e) {
          error.value = e?.message ?? String(e);
          plan.value = null;
        } finally {
          loading.value = false;
        }
      }, debounceMs);
    },
    { deep: true, immediate: true },
  );

  return { plan, error, loading };
}
