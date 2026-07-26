import type { Granularity } from "./api.js";

// Buckets shown per "page" of the chart, and the hard ceiling a user can
// widen a custom range to, keyed by granularity. Mirrors
// internal/store.MaxRangeBuckets on the Go side — the server independently
// enforces the same ceiling, so these are a UX nicety, not the sole guard.
export const PAGE_BUCKETS: Record<Granularity, number> = { day: 30, week: 12, month: 12 };
export const MAX_BUCKETS: Record<Granularity, number> = { day: 90, week: 52, month: 36 };

function parseISODate(iso: string): Date {
  const [y, m, d] = iso.slice(0, 10).split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
}

function toISODate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

// Shifts an ISO "YYYY-MM-DD" date by `count` buckets of the given
// granularity: day -> +/-count days, week -> +/-count*7 days, month ->
// +/-count calendar months. Month arithmetic resolves the target month first
// (from day 1, so it can't itself overflow) and only then clamps the
// original day-of-month into that month's actual length - plain
// `Date#setUTCMonth` would instead silently roll a day like the 31st over
// into the following month whenever the target month is shorter (e.g. Jan
// 31 + 1 month landing on Mar 3 instead of Feb 28).
export function addBuckets(iso: string, granularity: Granularity, count: number): string {
  const d = parseISODate(iso);
  if (granularity === "month") {
    const day = d.getUTCDate();
    const target = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + count, 1));
    const daysInTargetMonth = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate();
    target.setUTCDate(Math.min(day, daysInTargetMonth));
    return toISODate(target);
  }
  d.setUTCDate(d.getUTCDate() + (granularity === "week" ? count * 7 : count));
  return toISODate(d);
}

// Counts how many `granularity` buckets are needed to cover [fromIso, toIso]
// inclusive (e.g. a single day is 1 bucket; a Mon-Sun week is 1 bucket).
// Computed directly (not by stepping addBuckets) so it exactly matches
// internal/store.checkRange's bucket-count formula on the Go side, which
// counts calendar months/whole days rather than day-of-month-anchored
// steps - the two must agree, or the client-side MAX_BUCKETS clamp could
// let a range through that the server then rejects with ErrRangeTooLong.
export function bucketSpan(fromIso: string, toIso: string, granularity: Granularity): number {
  const f = parseISODate(fromIso);
  const t = parseISODate(toIso);
  if (granularity === "month") {
    return (t.getUTCFullYear() - f.getUTCFullYear()) * 12 + (t.getUTCMonth() - f.getUTCMonth()) + 1;
  }
  const days = Math.round((t.getTime() - f.getTime()) / 86_400_000);
  return granularity === "week" ? Math.floor(days / 7) + 1 : days + 1;
}

// Clamps [fromIso, toIso] to at most MAX_BUCKETS[granularity] buckets.
// anchor "from" keeps the range's start fixed and pulls `to` in; anchor "to"
// keeps the end fixed and pulls `from` in.
export function clampSpan(
  fromIso: string,
  toIso: string,
  granularity: Granularity,
  anchor: "from" | "to",
): { from: string; to: string } {
  const max = MAX_BUCKETS[granularity];
  if (bucketSpan(fromIso, toIso, granularity) <= max) return { from: fromIso, to: toIso };
  return anchor === "from"
    ? { from: fromIso, to: addBuckets(fromIso, granularity, max - 1) }
    : { from: addBuckets(toIso, granularity, -(max - 1)), to: toIso };
}

// Snaps an ISO date back to the first day of the bucket it falls in (month:
// the 1st; week: that ISO week's Monday, matching internal/store's
// `date(created_at, '-6 days', 'weekday 1')` bucketing; day: itself).
function bucketStart(iso: string, granularity: Granularity): string {
  if (granularity === "day") return iso;
  const d = parseISODate(iso);
  if (granularity === "month") {
    return toISODate(new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1)));
  }
  const isoDow = ((d.getUTCDay() + 6) % 7) + 1; // Mon=1 .. Sun=7
  d.setUTCDate(d.getUTCDate() - (isoDow - 1));
  return toISODate(d);
}

// The default "latest page" window: PAGE_BUCKETS[granularity] buckets ending
// at lastIso. `from` is snapped to the start of its bucket - otherwise the
// oldest visible bucket would silently undercount (e.g. a monthly window
// starting mid-month would exclude that month's first days even though the
// bucket's label still reads as the whole month).
export function defaultWindow(lastIso: string, granularity: Granularity): { from: string; to: string } {
  const to = lastIso.slice(0, 10);
  const from = addBuckets(to, granularity, -(PAGE_BUCKETS[granularity] - 1));
  return { from: bucketStart(from, granularity), to };
}

// Shifts [fromIso, toIso] forward/backward by its own width (so consecutive
// pages tile without gaps or overlap), clamped to never leave
// [boundFromIso, boundToIso].
export function pageWindow(
  fromIso: string,
  toIso: string,
  granularity: Granularity,
  direction: 1 | -1,
  boundFromIso: string,
  boundToIso: string,
): { from: string; to: string } {
  const width = bucketSpan(fromIso, toIso, granularity);
  let from = addBuckets(fromIso, granularity, direction * width);
  let to = addBuckets(toIso, granularity, direction * width);
  const bf = boundFromIso.slice(0, 10);
  const bt = boundToIso.slice(0, 10);
  if (to > bt) {
    to = bt;
    from = addBuckets(to, granularity, -(width - 1));
  }
  if (from < bf) {
    from = bf;
    to = addBuckets(from, granularity, width - 1);
    if (to > bt) to = bt;
  }
  return { from, to };
}
