import { LitElement, html } from "lit";
import { customElement, query, state } from "lit/decorators.js";
import { fetchUsage, type Dimension, type Granularity, type Usage } from "../lib/api.js";
import { getLocale, setLocale, t, type Locale } from "../lib/i18n.js";
import { defaultWindow, PAGE_BUCKETS } from "../lib/period.js";
import type { ToggleOption } from "./usage-toggle-group.js";
import type { PeriodBounds } from "./usage-period-control.js";
import type { SessionDetailModal } from "./session-detail-modal.js";
import type { ModelDetailModal } from "./model-detail-modal.js";
import "./usage-toggle-group.js";
import "./usage-period-control.js";
import "./usage-summary-cards.js";
import "./usage-chart.js";
import "./session-detail-modal.js";
import "./model-detail-modal.js";

const dimensionOptions = (): ToggleOption[] => [
  { value: "model", label: t("dimensionModel") },
  { value: "session", label: t("dimensionSession") },
];

const granularityOptions = (): ToggleOption[] => [
  { value: "day", label: t("granularityDay") },
  { value: "week", label: t("granularityWeek") },
  { value: "month", label: t("granularityMonth") },
];

const languageOptions = (): ToggleOption[] => [
  { value: "ja", label: t("languageJa") },
  { value: "en", label: t("languageEn") },
];

@customElement("usage-dashboard")
export class UsageDashboard extends LitElement {
  @state() dimension: Dimension = "model";
  @state() granularity: Granularity = "day";
  @state() usage: Usage | null = null;
  // "" means "no explicit window yet" - #loadUsage fetches the full history
  // and, if it's longer than one page, snaps these to the latest page.
  @state() from = "";
  @state() to = "";
  @state() bounds: PeriodBounds | null = null;

  @query("session-detail-modal") sessionModal!: SessionDetailModal;
  @query("model-detail-modal") modelModal!: ModelDetailModal;

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    this.#loadUsage();
  }

  // Bumped on every #loadUsage call so a stale in-flight request (e.g. from
  // rapidly toggling granularity) can detect it's been superseded and avoid
  // clobbering newer state with an out-of-order response.
  #loadSeq = 0;

  async #loadUsage() {
    const seq = ++this.#loadSeq;
    const { ok, data } = await fetchUsage(this.dimension, this.granularity, this.from, this.to);
    if (seq !== this.#loadSeq) return;
    if (!ok) return;
    let usage = data as Usage;
    // firstAt/lastAt always cover the whole history regardless of from/to
    // (see internal/store.Store.meta), so this is a stable source for the
    // period control's bounds and doesn't need re-deriving per fetch.
    this.bounds = { firstAt: usage.firstAt, lastAt: usage.lastAt };

    if (!this.from && !this.to && usage.lastAt && usage.buckets.length > PAGE_BUCKETS[this.granularity]) {
      const win = defaultWindow(usage.lastAt, this.granularity);
      const windowed = await fetchUsage(this.dimension, this.granularity, win.from, win.to);
      if (seq !== this.#loadSeq) return;
      if (windowed.ok) {
        this.from = win.from;
        this.to = win.to;
        usage = windowed.data as Usage;
      }
    }
    this.usage = usage;
  }

  #onDimensionChange(e: CustomEvent<{ value: string }>) {
    this.dimension = e.detail.value as Dimension;
    this.#loadUsage();
  }

  #onGranularityChange(e: CustomEvent<{ value: string }>) {
    this.granularity = e.detail.value as Granularity;
    // Page size differs per granularity (30 days vs. 12 weeks vs. 12
    // months) - drop any explicit window so #loadUsage re-derives a
    // sensible default window for the new granularity.
    this.from = "";
    this.to = "";
    this.#loadUsage();
  }

  #onPeriodChange(e: CustomEvent<{ from: string; to: string }>) {
    this.from = e.detail.from;
    this.to = e.detail.to;
    this.#loadUsage();
  }

  #onSessionClick(e: CustomEvent<{ sessionId: string }>) {
    this.sessionModal.openSession(e.detail.sessionId);
  }

  #onModelClick(e: CustomEvent<{ model: string }>) {
    this.modelModal.openModel(e.detail.model);
  }

  #onLanguageChange(e: CustomEvent<{ value: string }>) {
    setLocale(e.detail.value as Locale);
  }

  render() {
    return html`
      <div class="flex items-start justify-between gap-3 flex-wrap mb-5">
        <div class="flex gap-3 flex-wrap">
          <usage-toggle-group
            .options=${dimensionOptions()}
            .value=${this.dimension}
            @change=${(e: CustomEvent<{ value: string }>) => this.#onDimensionChange(e)}
          ></usage-toggle-group>
          <usage-toggle-group
            .options=${granularityOptions()}
            .value=${this.granularity}
            @change=${(e: CustomEvent<{ value: string }>) => this.#onGranularityChange(e)}
          ></usage-toggle-group>
        </div>
        <usage-toggle-group
          .options=${languageOptions()}
          .value=${getLocale()}
          @change=${(e: CustomEvent<{ value: string }>) => this.#onLanguageChange(e)}
        ></usage-toggle-group>
      </div>
      <div class="mb-5">
        <usage-period-control
          .granularity=${this.granularity}
          .from=${this.from}
          .to=${this.to}
          .bounds=${this.bounds}
          @change=${(e: CustomEvent<{ from: string; to: string }>) => this.#onPeriodChange(e)}
        ></usage-period-control>
      </div>
      <usage-summary-cards .usage=${this.usage}></usage-summary-cards>
      <usage-chart
        .usage=${this.usage}
        @session-click=${(e: CustomEvent<{ sessionId: string }>) => this.#onSessionClick(e)}
        @model-click=${(e: CustomEvent<{ model: string }>) => this.#onModelClick(e)}
      ></usage-chart>
      <session-detail-modal></session-detail-modal>
      <model-detail-modal></model-detail-modal>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "usage-dashboard": UsageDashboard;
  }
}
