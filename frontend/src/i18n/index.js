// i18n 入口：按 PRD 要求预留多语言结构，v1 仅提供 zh-CN。
// 如需新增语言：在本目录下新增 xx-XX.js，然后加入下方 messages。
import { createI18n } from 'vue-i18n';
import zhCN from './zh-CN.js';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
  },
});

export default i18n;
