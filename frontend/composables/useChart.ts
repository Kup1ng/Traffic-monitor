// Shared chart palette and helpers so the live and history charts share one
// look that matches the light claymorphism design system.
export function useChart() {
  const colors = {
    rx: '#3b9ef7',
    tx: '#ff9a5c',
    rxFill: 'rgba(59, 158, 247, 0.16)',
    txFill: 'rgba(255, 154, 92, 0.15)',
    grid: 'rgba(124, 110, 222, 0.14)',
    ticks: '#8b8bad',
    tooltipBg: '#ffffff',
    tooltipBorder: 'rgba(124, 110, 222, 0.22)',
    tooltipText: '#44425f',
    font: "'Fira Sans', ui-sans-serif, system-ui, sans-serif",
  }

  const prefersReduced = (): boolean =>
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  return { colors, prefersReduced }
}
