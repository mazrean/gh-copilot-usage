import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";
import type { Granularity } from "../lib/api.js";
import { clampSpan, defaultWindow, MAX_BUCKETS, pageWindow } from "../lib/period.js";
import { t } from "../lib/i18n.js";

export interface PeriodBounds {
  firstAt: string;
  lastAt: string;
}

const unitKey: Record<Granularity, "periodUnitDays" | "periodUnitWeeks" | "periodUnitMonths"> = {
  day: "periodUnitDays",
  week: "periodUnitWeeks",
  month: "periodUnitMonths",
};

// Lets the user narrow the chart to a bounded date range (clamped to
// MAX_BUCKETS[granularity], mirroring the server-side limit) and page
// through history a window at a time, so a long usage history never has to
// render every bucket at once. `from`/`to` are ISO "YYYY-MM-DD"; empty means
// "use the full data range" (`bounds`).
@customElement("usage-period-control")
export class UsagePeriodControl extends LitElement {
  @property({ type: String }) granularity: Granularity = "day";
  @property({ type: String }) from = "";
  @property({ type: String }) to = "";
  @property({ attribute: false }) bounds: PeriodBounds | null = null;

  createRenderRoot() {
    return this;
  }

  #boundFrom() {
    return this.bounds ? this.bounds.firstAt.slice(0, 10) : "";
  }

  #boundTo() {
    return this.bounds ? this.bounds.lastAt.slice(0, 10) : "";
  }

  #effFrom() {
    return this.from || this.#boundFrom();
  }

  #effTo() {
    return this.to || this.#boundTo();
  }

  #emit(win: { from: string; to: string }) {
    this.dispatchEvent(new CustomEvent("change", { detail: win, bubbles: true, composed: true }));
  }

  #onFromChange(e: Event) {
    const value = (e.target as HTMLInputElement).value;
    if (!value) return;
    const to = value > this.#effTo() ? value : this.#effTo();
    this.#emit(clampSpan(value, to, this.granularity, "from"));
  }

  #onToChange(e: Event) {
    const value = (e.target as HTMLInputElement).value;
    if (!value) return;
    const from = value < this.#effFrom() ? value : this.#effFrom();
    this.#emit(clampSpan(from, value, this.granularity, "to"));
  }

  #page(direction: 1 | -1) {
    if (!this.bounds) return;
    this.#emit(
      pageWindow(this.#effFrom(), this.#effTo(), this.granularity, direction, this.bounds.firstAt, this.bounds.lastAt),
    );
  }

  #latest() {
    if (!this.bounds) return;
    this.#emit(defaultWindow(this.bounds.lastAt, this.granularity));
  }

  render() {
    const effFrom = this.#effFrom();
    const effTo = this.#effTo();
    const boundFrom = this.#boundFrom();
    const boundTo = this.#boundTo();
    const prevDisabled = !this.bounds || effFrom <= boundFrom;
    const nextDisabled = !this.bounds || effTo >= boundTo;

    return html`
      <div class="period-control">
        <button
          type="button"
          class="period-nav-btn"
          ?disabled=${prevDisabled}
          title=${t("periodPrev")}
          aria-label=${t("periodPrev")}
          @click=${() => this.#page(-1)}
        >
          ‹
        </button>
        <input
          type="date"
          class="period-date-input"
          .value=${effFrom}
          min=${boundFrom || ""}
          max=${boundTo || ""}
          @change=${(e: Event) => this.#onFromChange(e)}
        />
        <span class="period-sep">${t("dateRangeSeparator")}</span>
        <input
          type="date"
          class="period-date-input"
          .value=${effTo}
          min=${boundFrom || ""}
          max=${boundTo || ""}
          @change=${(e: Event) => this.#onToChange(e)}
        />
        <button
          type="button"
          class="period-nav-btn"
          ?disabled=${nextDisabled}
          title=${t("periodNext")}
          aria-label=${t("periodNext")}
          @click=${() => this.#page(1)}
        >
          ›
        </button>
        <button type="button" class="toggle-btn toggle-btn-idle period-latest-btn" ?disabled=${nextDisabled} @click=${() => this.#latest()}>
          ${t("periodLatest")}
        </button>
        <span class="period-hint">${t("periodMaxHint", { count: MAX_BUCKETS[this.granularity], unit: t(unitKey[this.granularity]) })}</span>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "usage-period-control": UsagePeriodControl;
  }
}
