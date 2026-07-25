import { LitElement, html } from "lit";
import { customElement, property, state } from "lit/decorators.js";
import { fetchMonthly, type ApiError, type Monthly, type Usage } from "../lib/api.js";
import { computeMonthlyPace, formatAIU, formatDateRange } from "../lib/format.js";

@customElement("usage-summary-cards")
export class UsageSummaryCards extends LitElement {
  @property({ attribute: false }) usage: Usage | null = null;

  @state() monthly: Monthly | null = null;
  @state() monthlyError = "";

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
      this.monthlyError = (data as ApiError).error || "取得に失敗しました";
      return;
    }
    this.monthlyError = "";
    this.monthly = data as Monthly;
  }

  render() {
    const u = this.usage;
    const pace = this.monthly
      ? computeMonthlyPace(this.monthly.totalAIC, this.monthly.year, this.monthly.month)
      : null;

    return html`
      <div class="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-4 mb-5">
        <div class="group-box">
          <h2 class="group-title">ローカル計測</h2>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3">
            <div>
              <div class="card-label">ローカル総 AIU</div>
              <div class="card-value">${u ? formatAIU(u.totalAIU) : "-"}</div>
            </div>
            <div>
              <div class="card-label">記録件数</div>
              <div class="card-value">${u ? u.rows : "-"}</div>
            </div>
            <div>
              <div class="card-label">期間</div>
              <div class="card-value-small">${u ? formatDateRange(u.firstAt, u.lastAt) : "-"}</div>
            </div>
          </div>
        </div>
        <div class="group-box">
          <h2 class="group-title">今月の請求ベース</h2>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3">
            <div>
              <div class="card-label">今月 AIC</div>
              <div class="card-value">${this.monthly ? this.monthly.totalAIC.toFixed(2) : "-"}</div>
              <div class="card-error">${this.monthlyError}</div>
            </div>
            <div>
              <div class="card-label">経過日数</div>
              <div class="card-value-small">${pace ? `${pace.daysElapsed}/${pace.daysInMonth} 日` : "-"}</div>
            </div>
            <div>
              <div class="card-label">月末予測 AIC</div>
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
