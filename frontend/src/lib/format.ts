import { t } from "./i18n.js";

export function formatAIU(value: number): string {
  return value.toFixed(3);
}

// Converts a "yyyy-mm-dd..." prefix (ISO date or datetime) into "yyyy/mm/dd".
function toSlashDate(isoDate: string): string {
  return isoDate.slice(0, 10).replaceAll("-", "/");
}

export function formatDateRange(firstAt: string, lastAt: string): string {
  if (!firstAt || !lastAt) return "-";
  return `${toSlashDate(firstAt)}${t("dateRangeSeparator")}${toSlashDate(lastAt)}`;
}

export function formatTimestamp(value: string): string {
  return `${toSlashDate(value)} ${value.slice(11, 16)}`;
}

function addDaysUTC(isoDate: string, days: number): { year: number; month: number; day: number } {
  const [y, m, d] = isoDate.split("-").map(Number);
  const dt = new Date(Date.UTC(y, m - 1, d + days));
  return { year: dt.getUTCFullYear(), month: dt.getUTCMonth() + 1, day: dt.getUTCDate() };
}

const pad2 = (n: number) => String(n).padStart(2, "0");

// Formats an x-axis bucket key from /api/usage for display, given its granularity.
// - day:   "2026-07-25"                          -> "2026/07/25"
// - month: "2026-07"                              -> "2026/07"
// - week:  "2026-07-20" (Monday, start of week)  -> the Mon-Sun date range:
//            same month           -> "2026/07/20~26"
//            crosses month/year   -> "2026/07/27~08/02"
export function formatBucketLabel(bucket: string, granularity: "day" | "week" | "month"): string {
  if (granularity === "day") return toSlashDate(bucket);
  if (granularity === "month") return bucket.replaceAll("-", "/");

  const [startY, startM, startD] = bucket.split("-").map(Number);
  const end = addDaysUTC(bucket, 6);
  const start = `${startY}/${pad2(startM)}/${pad2(startD)}`;
  if (end.year === startY && end.month === startM) {
    return `${start}~${pad2(end.day)}`;
  }
  return `${start}~${pad2(end.month)}/${pad2(end.day)}`;
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
