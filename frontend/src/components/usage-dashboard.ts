import { LitElement, html } from "lit";
import { customElement, query, state } from "lit/decorators.js";
import { fetchUsage, type Dimension, type Granularity, type Usage } from "../lib/api.js";
import type { ToggleOption } from "./usage-toggle-group.js";
import type { SessionDetailModal } from "./session-detail-modal.js";
import type { ModelDetailModal } from "./model-detail-modal.js";
import "./usage-toggle-group.js";
import "./usage-summary-cards.js";
import "./usage-chart.js";
import "./session-detail-modal.js";
import "./model-detail-modal.js";

const DIMENSION_OPTIONS: ToggleOption[] = [
  { value: "model", label: "モデル別" },
  { value: "session", label: "セッション別" },
];

const GRANULARITY_OPTIONS: ToggleOption[] = [
  { value: "day", label: "日次" },
  { value: "week", label: "週次" },
  { value: "month", label: "月次" },
];

@customElement("usage-dashboard")
export class UsageDashboard extends LitElement {
  @state() dimension: Dimension = "model";
  @state() granularity: Granularity = "day";
  @state() usage: Usage | null = null;

  @query("session-detail-modal") sessionModal!: SessionDetailModal;
  @query("model-detail-modal") modelModal!: ModelDetailModal;

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    this.#loadUsage();
  }

  async #loadUsage() {
    const { ok, data } = await fetchUsage(this.dimension, this.granularity);
    if (ok) this.usage = data as Usage;
  }

  #onDimensionChange(e: CustomEvent<{ value: string }>) {
    this.dimension = e.detail.value as Dimension;
    this.#loadUsage();
  }

  #onGranularityChange(e: CustomEvent<{ value: string }>) {
    this.granularity = e.detail.value as Granularity;
    this.#loadUsage();
  }

  #onSessionClick(e: CustomEvent<{ sessionId: string }>) {
    this.sessionModal.openSession(e.detail.sessionId);
  }

  #onModelClick(e: CustomEvent<{ model: string }>) {
    this.modelModal.openModel(e.detail.model);
  }

  render() {
    return html`
      <div class="flex gap-3 flex-wrap mb-5">
        <usage-toggle-group
          .options=${DIMENSION_OPTIONS}
          .value=${this.dimension}
          @change=${(e: CustomEvent<{ value: string }>) => this.#onDimensionChange(e)}
        ></usage-toggle-group>
        <usage-toggle-group
          .options=${GRANULARITY_OPTIONS}
          .value=${this.granularity}
          @change=${(e: CustomEvent<{ value: string }>) => this.#onGranularityChange(e)}
        ></usage-toggle-group>
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
