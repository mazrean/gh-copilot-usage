import { LitElement, html, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { fetchModelDetail, type ApiError, type ModelDetail } from "../lib/api.js";
import { formatAIU } from "../lib/format.js";
import { t } from "../lib/i18n.js";
import "./breakdown-pie-chart.js";

const categoryLabels = (): Record<string, string> => ({
  input: t("categoryInput"),
  cache_read: t("categoryCacheRead"),
  cache_write: t("categoryCacheWrite"),
  output: t("categoryOutput"),
  other: t("categoryOther"),
});

@customElement("model-detail-modal")
export class ModelDetailModal extends LitElement {
  @state() open = false;
  @state() loading = false;
  @state() error = "";
  @state() detail: ModelDetail | null = null;

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    document.addEventListener("keydown", this.#onKeydown);
  }

  disconnectedCallback() {
    document.removeEventListener("keydown", this.#onKeydown);
    super.disconnectedCallback();
  }

  #onKeydown = (e: KeyboardEvent) => {
    if (e.key === "Escape") this.close();
  };

  async openModel(model: string) {
    this.open = true;
    this.loading = true;
    this.error = "";
    this.detail = null;
    try {
      const { ok, data } = await fetchModelDetail(model);
      if (!ok) {
        this.error = (data as ApiError).error || t("fetchFailed");
      } else {
        this.detail = data as ModelDetail;
      }
    } catch {
      this.error = t("fetchFailed");
    } finally {
      this.loading = false;
    }
  }

  close() {
    this.open = false;
  }

  #onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) this.close();
  }

  render() {
    if (!this.open) return nothing;
    const d = this.detail;

    return html`
      <div class="modal-backdrop" role="presentation" @click=${(e: MouseEvent) => this.#onBackdropClick(e)}>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="model-modal-title">
          <div class="modal-header">
            <h2 id="model-modal-title" class="text-[1rem] font-semibold m-0">${d ? d.model : t("modelDetailTitle")}</h2>
            <button type="button" class="modal-close" aria-label=${t("close")} @click=${() => this.close()}>×</button>
          </div>
          <div class="modal-body">
            ${this.loading ? html`${t("loading")}` : nothing}
            ${!this.loading && this.error ? html`${this.error}` : nothing}
            ${!this.loading && d ? this.#renderDetail(d) : nothing}
          </div>
        </div>
      </div>
    `;
  }

  #renderDetail(d: ModelDetail) {
    const byCategory = (d.byCategory ?? []).filter((c) => c.aiu > 0);
    const labels = categoryLabels();

    return html`
      <dl class="m-0 mb-3 text-[0.85rem]">
        <div class="flex gap-2.5 py-[3px]">
          <dt class="flex-[0_0_7em] text-muted whitespace-nowrap">${t("total")}</dt>
          <dd class="m-0">${formatAIU(d.aiu)} ${t("unitAIU")} (${t("rowsSuffix", { count: d.rows })})</dd>
        </div>
      </dl>
      <div class="modal-section-title">${t("categoryBreakdown")}</div>
      ${d.byCategory === null
        ? html`<div>${t("noCategoryData")}</div>`
        : byCategory.length
          ? html`
              <breakdown-pie-chart
                .items=${byCategory.map((c) => ({ label: labels[c.category] ?? c.category, value: c.aiu }))}
              ></breakdown-pie-chart>
            `
          : html`<div>${t("noData")}</div>`}
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "model-detail-modal": ModelDetailModal;
  }
}
