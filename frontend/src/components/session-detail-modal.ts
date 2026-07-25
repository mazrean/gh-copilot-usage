import { LitElement, html, nothing } from "lit";
import { customElement, state } from "lit/decorators.js";
import {
  fetchSessionDetail,
  type ApiError,
  type SessionDetail,
  type SessionModelUsage,
  type SessionTurnUsage,
} from "../lib/api.js";
import { formatAIU, formatDurationMs, formatTimestamp, formatTokenCount } from "../lib/format.js";
import "./turn-usage-chart.js";

@customElement("session-detail-modal")
export class SessionDetailModal extends LitElement {
  @state() open = false;
  @state() loading = false;
  @state() error = "";
  @state() detail: SessionDetail | null = null;
  @state() selectedTurnIndex: number | null = null;

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

  async openSession(sessionId: string) {
    this.open = true;
    this.loading = true;
    this.error = "";
    this.detail = null;
    this.selectedTurnIndex = null;
    try {
      const { ok, data } = await fetchSessionDetail(sessionId);
      if (!ok) {
        this.error = (data as ApiError).error || "取得に失敗しました";
      } else {
        this.detail = data as SessionDetail;
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

  #onTurnClick(e: CustomEvent<{ turnIndex: number }>) {
    this.selectedTurnIndex = e.detail.turnIndex;
  }

  render() {
    if (!this.open) return nothing;
    const d = this.detail;

    return html`
      <div class="modal-backdrop" role="presentation" @click=${(e: MouseEvent) => this.#onBackdropClick(e)}>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="session-modal-title">
          <div class="modal-header">
            <h2 id="session-modal-title" class="text-[1rem] font-semibold m-0">
              ${d ? d.summary || d.id : "セッション詳細"}
            </h2>
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

  #renderDetail(d: SessionDetail) {
    const byModel = d.byModel ?? [];
    const turns = d.turns ?? [];
    const metaRows: Array<[string, string, string?]> = [];
    if (d.repository) {
      metaRows.push(["リポジトリ", d.branch ? `${d.repository} (${d.branch})` : d.repository]);
    }
    if (d.cwd) metaRows.push(["作業ディレクトリ", d.cwd, "font-mono"]);
    if (d.createdAt) metaRows.push(["作成", formatTimestamp(d.createdAt)]);
    if (d.updatedAt) metaRows.push(["更新", formatTimestamp(d.updatedAt)]);

    return html`
      <dl class="m-0 mb-3 text-[0.85rem]">
        ${metaRows.map(
          ([label, value, cls]) => html`
            <div class="flex gap-2.5 py-[3px]">
              <dt class="flex-[0_0_7em] text-muted whitespace-nowrap">${label}</dt>
              <dd class="m-0 [overflow-wrap:anywhere] ${cls ?? ""}">${value}</dd>
            </div>
          `,
        )}
      </dl>
      <div class="modal-section-title">モデル別内訳</div>
      <ul class="list-none m-0 p-0 text-[0.9rem]">
        ${byModel.length
          ? byModel.map(
              (m) => html`
                <li class="flex justify-between gap-3 py-1 border-b border-border">
                  <span>${m.model}</span>
                  <span>${formatAIU(m.aiu)} AIU (${m.rows}件)</span>
                </li>
              `,
            )
          : html`<li>データがありません</li>`}
      </ul>
      <div class="modal-section-title">ターン別内訳</div>
      ${turns.length
        ? html`
            <turn-usage-chart
              .turns=${turns}
              @turn-click=${(e: CustomEvent<{ turnIndex: number }>) => this.#onTurnClick(e)}
            ></turn-usage-chart>
            ${this.#renderSelectedTurn(turns)}
          `
        : html`<div>ターンの記録はありません</div>`}
    `;
  }

  #renderSelectedTurn(turns: SessionTurnUsage[]) {
    if (this.selectedTurnIndex === null) {
      return html`<div class="text-[0.82rem] text-muted mt-2">グラフ内のターンをクリックすると詳細が表示されます</div>`;
    }
    const t = turns.find((turn) => turn.turnIndex === this.selectedTurnIndex);
    if (!t) return nothing;

    const latencyParts: string[] = [];
    if (t.durationMs) latencyParts.push(`所要時間 ${formatDurationMs(t.durationMs)}`);
    if (t.timeToFirstTokenMs) latencyParts.push(`初回応答 ${formatDurationMs(t.timeToFirstTokenMs)}`);
    if (t.finishReason) latencyParts.push(`終了理由 ${t.finishReason}`);

    return html`
      <div class="turn-item mt-2">
        <div class="font-semibold text-[0.9rem]">
          ${t.turnIndex >= 0 ? `ターン #${t.turnIndex}` : "未割当"}
          <span class="font-normal text-muted">— ${formatAIU(t.aiu)} AIU</span>
        </div>
        ${t.timestamp ? html`<div class="text-[0.78rem] text-muted mt-1">${formatTimestamp(t.timestamp)}</div>` : nothing}
        ${latencyParts.length
          ? html`<div class="text-[0.78rem] text-muted mt-1">${latencyParts.join(" ／ ")}</div>`
          : nothing}
        ${this.#renderTokenBreakdown(t.byModel)}
        ${t.userMessage
          ? html`<div class="text-[0.82rem] text-muted mt-2 [white-space:pre-wrap]">${t.userMessage}</div>`
          : nothing}
        ${t.assistantResponse
          ? html`
              <details class="mt-2">
                <summary class="text-[0.82rem] cursor-pointer">アシスタント応答</summary>
                <div class="text-[0.82rem] text-muted mt-1 [white-space:pre-wrap]">${t.assistantResponse}</div>
              </details>
            `
          : nothing}
      </div>
    `;
  }

  #renderTokenBreakdown(byModel: SessionModelUsage[]) {
    const withTokens = byModel.filter((m) => m.tokens);
    if (!withTokens.length) return nothing;

    return html`
      <table class="w-full text-[0.78rem] mt-2 border-collapse">
        <thead>
          <tr class="text-muted">
            <th class="text-left font-normal">モデル</th>
            <th class="text-right font-normal">input</th>
            <th class="text-right font-normal">output</th>
            <th class="text-right font-normal">cache_read</th>
            <th class="text-right font-normal">cache_write</th>
            <th class="text-right font-normal">reasoning</th>
          </tr>
        </thead>
        <tbody>
          ${withTokens.map((m) => {
            const tok = m.tokens!;
            return html`
              <tr class="border-t border-border">
                <td class="py-1">${m.model}</td>
                <td class="text-right">${formatTokenCount(tok.inputTokens)}</td>
                <td class="text-right">${formatTokenCount(tok.outputTokens)}</td>
                <td class="text-right">${formatTokenCount(tok.cacheReadTokens)}</td>
                <td class="text-right">${formatTokenCount(tok.cacheWriteTokens)}</td>
                <td class="text-right">${formatTokenCount(tok.reasoningTokens)}</td>
              </tr>
            `;
          })}
        </tbody>
      </table>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "session-detail-modal": SessionDetailModal;
  }
}
