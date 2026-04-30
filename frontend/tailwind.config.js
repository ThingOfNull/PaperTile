/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts}'],
  theme: {
    extend: {
      colors: {
        fluent: {
          // Windows 11 Fluent 蓝色系。
          50:  '#eff6ff',
          100: '#dbeafe',
          200: '#bfdbfe',
          300: '#93c5fd',
          400: '#60a5fa',
          500: '#0078d4', // Fluent 主蓝
          600: '#106ebe',
          700: '#005a9e',
          800: '#004578',
          900: '#002b4b',
        },
      },
      boxShadow: {
        // Fluent Design 的轻微阴影层级。
        fluent1: '0 1px 2px rgba(0,0,0,0.04), 0 2px 4px rgba(0,0,0,0.06)',
        fluent2: '0 2px 4px rgba(0,0,0,0.06), 0 4px 8px rgba(0,0,0,0.08)',
      },
      borderRadius: {
        fluent: '6px',
      },
      fontFamily: {
        sans: [
          '"Segoe UI Variable"',
          '"Segoe UI"',
          '"Microsoft YaHei UI"',
          'system-ui',
          'sans-serif',
        ],
      },
    },
  },
  plugins: [],
};
