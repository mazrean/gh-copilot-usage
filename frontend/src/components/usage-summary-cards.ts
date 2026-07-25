import { LitElement, html } from "lit";
import { customElement, property, state } from "lit/decorators.js";
import { fetchMonthly, type ApiError, type Monthly, type Usage } from "../lib/api.js";
import { computeMonthlyPace, formatAIU, formatDateRange, type PaceMode } from "../lib/format.js";
import { t } from "../lib/i18n.js";
import type { ToggleOption } from "./usage-toggle-group.js";
import "./usage-toggle-group.js";

const paceModeOptions = (): ToggleOption[] => [
  { value: "calendar", label: t("paceModeCalendar") },
  { value: "weekday", label: t("paceModeWeekday") },
];

@customElement("usage-summary-cards")
export class UsageSummaryCards extends LitElement {
  @property({ attribute: false }) usage: Usage | null = null;

  @state() monthly: Monthly | null = null;
  @state() monthlyError = "";
  @state() paceMode: PaceMode = "calendar";

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    this.#loadMonthly();
  }

  async #loadMonthly() {
    const { ok, data } = await fetchMonthly();
    if (!ok) {
      this.monthly = null;
      this.monthlyError = (data as ApiError).error || t("fetchFailed");
      return;
    }
    this.monthlyError = "";
    this.monthly = data as Monthly;
  }

  render() {
    const u = this.usage;
    const pace = this.monthly
      ? computeMonthlyPace(this.monthly.totalAIC, this.monthly.year, this.monthly.month, this.paceMode)
      : null;

    return html`
      <div class="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-4 mb-5">
        <div class="group-box">
          <h2 class="group-title">${t("summaryLocal")}</h2>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3">
            <div>
              <div class="card-label">${t("summaryLocalTotalAIU")}</div>
              <div class="card-value">${u ? formatAIU(u.totalAIU) : "-"}</div>
            </div>
            <div>
              <div class="card-label">${t("summaryRecordCount")}</div>
              <div class="card-value">${u ? u.rows : "-"}</div>
            </div>
            <div>
              <div class="card-label">${t("summaryPeriod")}</div>
              <div class="card-value-small">${u ? formatDateRange(u.firstAt, u.lastAt) : "-"}</div>
            </div>
          </div>
        </div>
        <div class="group-box">
          <div class="flex items-center justify-between gap-3 flex-wrap">
            <h2 class="group-title">${t("summaryBillingThisMonth")}</h2>
            <usage-toggle-group
              .options=${paceModeOptions()}
              .value=${this.paceMode}
              @change=${(e: CustomEvent<{ value: string }>) => {
                this.paceMode = e.detail.value as PaceMode;
              }}
            ></usage-toggle-group>
          </div>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3">
            <div>
              <div class="card-label">${t("summaryMonthlyAIC")}</div>
              <div class="card-value">${this.monthly ? this.monthly.totalAIC.toFixed(2) : "-"}</div>
              <div class="card-error">${this.monthlyError}</div>
            </div>
            <div>
              <div class="card-label">
                ${this.paceMode === "weekday" ? t("summaryElapsedWeekdays") : t("summaryElapsedDays")}
              </div>
              <div class="card-value-small">
                ${pace ? `${pace.daysElapsed}/${pace.daysInMonth} ${t("summaryDaysUnit")}` : "-"}
              </div>
            </div>
            <div>
              <div class="card-label">${t("summaryProjectedAIC")}</div>
              <div class="card-value">${pace ? pace.projected.toFixed(2) : "-"}</div>
            </div>
          </div>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "usage-summary-cards": UsageSummaryCards;
  }
}
