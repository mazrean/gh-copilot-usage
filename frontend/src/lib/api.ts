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

export interface TurnTokenUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  reasoningTokens: number;
}

export interface SessionModelUsage {
  model: string;
  aiu: number;
  rows: number;
  // present only when the session-store.db exposes per-event detail columns.
  tokens?: TurnTokenUsage;
}

export interface SessionTurnUsage {
  turnIndex: number;
  aiu: number;
  userMessage: string;
  // the following are present only when the session-store.db exposes the
  // corresponding columns (older schemas omit them).
  assistantResponse?: string;
  timestamp?: string;
  durationMs?: number;
  timeToFirstTokenMs?: number;
  finishReason?: string;
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

export interface ModelCategoryUsage {
  category: string;
  aiu: number;
}

export interface ModelDetail {
  model: string;
  aiu: number;
  rows: number;
  // null when the underlying session-store.db predates per-category cost
  // breakdown (token_details_json column).
  byCategory: ModelCategoryUsage[] | null;
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

export function fetchModelDetail(model: string) {
  return fetchJSON<ModelDetail>(`/api/model?name=${encodeURIComponent(model)}`);
}

export function fetchMonthly() {
  return fetchJSON<Monthly>("/api/monthly");
}
