import { LitElement, html, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import { fetchModelDetail, type ApiError, type ModelDetail } from "../lib/api.js";
import { formatAIU } from "../lib/format.js";
import "./breakdown-pie-chart.js";

const CATEGORY_LABELS: Record<string, string> = {
  input: "入力",
  cache_read: "キャッシュ済み入力",
  cache_write: "キャッシュ書き込み",
  output: "出力",
  other: "その他",
};

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
        this.error = (data as ApiError).error || "取得に失敗しました";
      } else {
        this.detail = data as ModelDetail;
      }
    } catch {
      this.error = "取得に失敗しました";
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
            <h2 id="model-modal-title" class="text-[1rem] font-semibold m-0">${d ? d.model : "モデル詳細"}</h2>
            <button type="button" class="modal-close" aria-label="閉じる" @click=${() => this.close()}>×</button>
          </div>
          <div class="modal-body">
            ${this.loading ? html`読み込み中……` : nothing}
            ${!this.loading && this.error ? html`${this.error}` : nothing}
            ${!this.loading && d ? this.#renderDetail(d) : nothing}
          </div>
        </div>
      </div>
    `;
  }

  #renderDetail(d: ModelDetail) {
    const byCategory = (d.byCategory ?? []).filter((c) => c.aiu > 0);

    return html`
      <dl class="m-0 mb-3 text-[0.85rem]">
        <div class="flex gap-2.5 py-[3px]">
          <dt class="flex-[0_0_7em] text-muted whitespace-nowrap">合計</dt>
          <dd class="m-0">${formatAIU(d.aiu)} AIU (${d.rows}件)</dd>
        </div>
      </dl>
      <div class="modal-section-title">トークン種別内訳</div>
      ${d.byCategory === null
        ? html`<div>このデータには入力/出力などの内訳情報がありません</div>`
        : byCategory.length
          ? html`
              <breakdown-pie-chart
                .items=${byCategory.map((c) => ({ label: CATEGORY_LABELS[c.category] ?? c.category, value: c.aiu }))}
              ></breakdown-pie-chart>
            `
          : html`<div>データがありません</div>`}
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "model-detail-modal": ModelDetailModal;
  }
}
