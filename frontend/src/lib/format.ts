export function formatAIU(value: number): string {
  return value.toFixed(3);
}

export function formatDateRange(firstAt: string, lastAt: string): string {
  if (!firstAt || !lastAt) return "-";
  return `${firstAt.slice(0, 10)} 〜 ${lastAt.slice(0, 10)}`;
}

export function formatTimestamp(value: string): string {
  return value.slice(0, 16).replace("T", " ");
}

export function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatTokenCount(n: number): string {
  return n.toLocaleString();
}

export interface MonthlyPace {
  daysElapsed: number;
  daysInMonth: number;
  projected: number;
}

export function computeMonthlyPace(totalAIC: number, year: number, month: number): MonthlyPace {
  const daysInMonth = new Date(year, month, 0).getDate();
  const now = new Date();
  const isCurrentMonth = now.getFullYear() === year && now.getMonth() + 1 === month;
  const daysElapsed = isCurrentMonth ? now.getDate() : daysInMonth;
  const projected = daysElapsed > 0 ? (totalAIC / daysElapsed) * daysInMonth : totalAIC;
  return { daysElapsed, daysInMonth, projected };
}
