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

export type PaceMode = "calendar" | "weekday";

function countWeekdaysThrough(year: number, month: number, lastDay: number): number {
  let count = 0;
  for (let d = 1; d <= lastDay; d++) {
    const dow = new Date(year, month - 1, d).getDay();
    if (dow !== 0 && dow !== 6) count++;
  }
  return count;
}

export function computeMonthlyPace(
  totalAIC: number,
  year: number,
  month: number,
  mode: PaceMode = "calendar",
): MonthlyPace {
  const daysInMonth = new Date(year, month, 0).getDate();
  const now = new Date();
  const isCurrentMonth = now.getFullYear() === year && now.getMonth() + 1 === month;
  const calendarDaysElapsed = isCurrentMonth ? now.getDate() : daysInMonth;

  if (mode === "weekday") {
    const weekdaysInMonth = countWeekdaysThrough(year, month, daysInMonth);
    const weekdaysElapsed = countWeekdaysThrough(year, month, calendarDaysElapsed);
    const projected = weekdaysElapsed > 0 ? (totalAIC / weekdaysElapsed) * weekdaysInMonth : totalAIC;
    return { daysElapsed: weekdaysElapsed, daysInMonth: weekdaysInMonth, projected };
  }

  const projected = calendarDaysElapsed > 0 ? (totalAIC / calendarDaysElapsed) * daysInMonth : totalAIC;
  return { daysElapsed: calendarDaysElapsed, daysInMonth, projected };
}
