export type Dimension = "model" | "session";
export type Granularity = "day" | "week" | "month";

export interface Series {
  key: string;
  label: string;
  values: number[];
}

export interface Usage {
  unit: string;
  dimension: Dimension;
  granularity: Granularity;
  buckets: string[];
  series: Series[];
  totalAIU: number;
  rows: number;
  firstAt: string;
  lastAt: string;
}

export interface SessionModelUsage {
  model: string;
  aiu: number;
  rows: number;
}

export interface SessionTurnUsage {
  turnIndex: number;
  aiu: number;
  userMessage: string;
  byModel: SessionModelUsage[];
}

export interface SessionDetail {
  id: string;
  summary: string;
  repository: string;
  branch: string;
  cwd: string;
  createdAt: string;
  updatedAt: string;
  // Go serializes a nil slice as `null`, which happens whenever a session
  // has no usage rows / turns recorded.
  byModel: SessionModelUsage[] | null;
  turns: SessionTurnUsage[] | null;
}

export interface ModelUsage {
  model: string;
  sku: string;
  netQuantity: number;
  netAmount: number;
  unitType: string;
}

export interface Monthly {
  login: string;
  year: number;
  month: number;
  totalAIC: number;
  byModel: ModelUsage[] | null;
}

export interface ApiError {
  error: string;
}

async function fetchJSON<T>(url: string): Promise<{ ok: boolean; status: number; data: T | ApiError }> {
  const res = await fetch(url);
  const data = await res.json();
  return { ok: res.ok, status: res.status, data };
}

export function fetchUsage(dimension: Dimension, granularity: Granularity) {
  const params = new URLSearchParams({ dimension, granularity });
  return fetchJSON<Usage>(`/api/usage?${params}`);
}

export function fetchSessionDetail(id: string) {
  return fetchJSON<SessionDetail>(`/api/session?id=${encodeURIComponent(id)}`);
}

export function fetchMonthly() {
  return fetchJSON<Monthly>("/api/monthly");
}
