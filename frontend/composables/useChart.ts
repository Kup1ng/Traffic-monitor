// Shared chart palette and helpers so the live and history charts share one
// look that matches the dark design system.
export function useChart() {
  const colors = {
    rx: '#38bdf8',
    tx: '#fb923c',
    rxFill: 'rgba(56, 189, 248, 0.18)',
    txFill: 'rgba(251, 146, 60, 0.16)',
    grid: 'rgba(40, 49, 71, 0.55)',
    ticks: '#94a3b8',
    tooltipBg: '#0b0f19',
    tooltipBorder: '#283147',
    font: "'Fira Sans', ui-sans-serif, system-ui, sans-serif",
  }

  const prefersReduced = (): boolean =>
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  return { colors, prefersReduced }
}
