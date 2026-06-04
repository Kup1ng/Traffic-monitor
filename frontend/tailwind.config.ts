import type { Config } from 'tailwindcss'
import primeui from 'tailwindcss-primeui'

export default <Partial<Config>>{
  darkMode: ['selector', '.dark'],
  content: [
    './components/**/*.{vue,js,ts}',
    './layouts/**/*.vue',
    './pages/**/*.vue',
    './composables/**/*.{js,ts}',
    './app.vue',
    './error.vue',
  ],
  theme: {
    extend: {
      colors: {
        bg: 'var(--bg)',
        surface: 'var(--surface)',
        'surface-2': 'var(--surface-2)',
        edge: 'var(--border)',
        ink: 'var(--text)',
        muted: 'var(--text-muted)',
        rx: 'var(--rx)',
        tx: 'var(--tx)',
        brand: 'var(--brand)',
        ok: 'var(--up)',
        warn: 'var(--warn)',
        down: 'var(--down)',
      },
      fontFamily: {
        sans: ['Fira Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['Fira Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
    },
  },
  plugins: [primeui],
}
