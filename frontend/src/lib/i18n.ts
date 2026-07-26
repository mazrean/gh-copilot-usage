export const SUPPORTED_LOCALES = ["ja", "en"] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];

const STORAGE_KEY = "gh-copilot-usage-locale";

const en = {
  appTitle: "GitHub Copilot AIC Usage",
  docTitle: "Copilot AIC Usage",
  dimensionModel: "By model",
  dimensionSession: "By session",
  granularityDay: "Daily",
  granularityWeek: "Weekly",
  granularityMonth: "Monthly",
  paceModeCalendar: "Calendar days",
  paceModeWeekday: "Weekdays only",
  languageJa: "日本語",
  languageEn: "English",
  fetchFailed: "Failed to fetch",
  unitAIU: "AIU",
  summaryLocal: "Local measurement",
  summaryLocalTotalAIU: "Local total AIU",
  summaryRecordCount: "Record count",
  summaryPeriod: "Period",
  summaryBillingThisMonth: "This month (billing)",
  summaryMonthlyAIC: "This month's AIC",
  summaryElapsedWeekdays: "Weekdays elapsed",
  summaryElapsedDays: "Days elapsed",
  summaryDaysUnit: "days",
  summaryProjectedAIC: "Projected month-end AIC",
  sessionDetailTitle: "Session detail",
  modelDetailTitle: "Model detail",
  close: "Close",
  loading: "Loading…",
  repository: "Repository",
  cwd: "Working directory",
  createdAt: "Created",
  updatedAt: "Updated",
  byModelBreakdown: "Breakdown by model",
  noData: "No data",
  byTurnBreakdown: "Breakdown by turn",
  noTurns: "No turns recorded",
  turnClickHint: "Click a turn in the chart to see its details",
  unassigned: "Unassigned",
  turnLabel: "Turn #{index}",
  durationLabel: "Duration: {value}",
  listSeparator: " / ",
  assistantResponse: "Assistant response",
  tokenTableModel: "Model",
  total: "Total",
  rowsSuffix: "{count} rows",
  categoryBreakdown: "Breakdown by token category",
  noCategoryData: "This data has no input/output breakdown available",
  categoryInput: "Input",
  categoryCacheRead: "Cached input",
  categoryCacheWrite: "Cache write",
  categoryOutput: "Output",
  categoryOther: "Other",
  traceMainAgent: "Main agent",
  traceInitiator: "initiator: {value}",
  dateRangeSeparator: " – ",
  periodPrev: "Previous period",
  periodNext: "Next period",
  periodLatest: "Latest",
  periodMaxHint: "Up to {count} {unit} per view",
  periodUnitDays: "days",
  periodUnitWeeks: "weeks",
  periodUnitMonths: "months",
} as const;

type TranslationKey = keyof typeof en;

const ja: Record<TranslationKey, string> = {
  appTitle: "GitHub Copilot AIC 使用量",
  docTitle: "Copilot AIC Usage",
  dimensionModel: "モデル別",
  dimensionSession: "セッション別",
  granularityDay: "日次",
  granularityWeek: "週次",
  granularityMonth: "月次",
  paceModeCalendar: "暦日ベース",
  paceModeWeekday: "平日のみ",
  languageJa: "日本語",
  languageEn: "English",
  fetchFailed: "取得に失敗しました",
  unitAIU: "AIU",
  summaryLocal: "ローカル計測",
  summaryLocalTotalAIU: "ローカル総 AIU",
  summaryRecordCount: "記録件数",
  summaryPeriod: "期間",
  summaryBillingThisMonth: "今月の請求ベース",
  summaryMonthlyAIC: "今月 AIC",
  summaryElapsedWeekdays: "経過平日数",
  summaryElapsedDays: "経過日数",
  summaryDaysUnit: "日",
  summaryProjectedAIC: "月末予測 AIC",
  sessionDetailTitle: "セッション詳細",
  modelDetailTitle: "モデル詳細",
  close: "閉じる",
  loading: "読み込み中……",
  repository: "リポジトリ",
  cwd: "作業ディレクトリ",
  createdAt: "作成",
  updatedAt: "更新",
  byModelBreakdown: "モデル別内訳",
  noData: "データがありません",
  byTurnBreakdown: "ターン別内訳",
  noTurns: "ターンの記録はありません",
  turnClickHint: "グラフ内のターンをクリックすると詳細が表示されます",
  unassigned: "未割当",
  turnLabel: "ターン #{index}",
  durationLabel: "所要時間: {value}",
  listSeparator: " ／ ",
  assistantResponse: "アシスタント応答",
  tokenTableModel: "モデル",
  total: "合計",
  rowsSuffix: "{count}件",
  categoryBreakdown: "トークン種別内訳",
  noCategoryData: "このデータには入力/出力などの内訳情報がありません",
  categoryInput: "入力",
  categoryCacheRead: "キャッシュ済み入力",
  categoryCacheWrite: "キャッシュ書き込み",
  categoryOutput: "出力",
  categoryOther: "その他",
  traceMainAgent: "メインエージェント",
  traceInitiator: "initiator: {value}",
  dateRangeSeparator: " 〜 ",
  periodPrev: "前の期間",
  periodNext: "次の期間",
  periodLatest: "最新",
  periodMaxHint: "一度に表示できるのは最大{count}{unit}",
  periodUnitDays: "日",
  periodUnitWeeks: "週",
  periodUnitMonths: "ヶ月",
};

const dictionaries: Record<Locale, Record<TranslationKey, string>> = { ja, en };

function isLocale(value: string | null): value is Locale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value ?? "");
}

function detectLocale(): Locale {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (isLocale(stored)) return stored;
  return navigator.language.toLowerCase().startsWith("ja") ? "ja" : "en";
}

let currentLocale: Locale = detectLocale();

export function getLocale(): Locale {
  return currentLocale;
}

export function setLocale(locale: Locale) {
  if (locale === currentLocale) return;
  currentLocale = locale;
  localStorage.setItem(STORAGE_KEY, locale);
  location.reload();
}

export function t(key: TranslationKey, params?: Record<string, string | number>): string {
  const template = dictionaries[currentLocale][key];
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ""));
}
