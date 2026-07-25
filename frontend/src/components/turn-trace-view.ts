import { LitElement, html, nothing } from "lit";
import { customElement, property, state } from "lit/decorators.js";
import type { TurnEventSpan } from "../lib/api.js";
import { formatAIU, formatDurationMs } from "../lib/format.js";
import { PALETTE } from "../lib/colors.js";

// Minimum visible width for a span whose AIU share of the turn would
// otherwise round to an invisible sliver.
const MIN_WIDTH_PCT = 1.5;

// Fixed-pixel gap carved out of each bar's left/right edge so consecutive
// same-color spans (e.g. two calls in a row by the same agent/model) stay
// visually distinguishable instead of blending into one block.
const GAP_PX = 2;

interface PositionedSpan {
  span: TurnEventSpan;
  leftPct: number;
  widthPct: number;
  color: string;
}

interface Lane {
  key: string;
  label: string;
  depth: number;
  items: PositionedSpan[];
}

/**
 * Renders a turn's assistant_usage_events as a waterfall ordered by call
 * sequence, with each bar's position/width driven by its share of the
 * turn's total AIU (not wall-clock time). Spans made by a delegated
 * sub-agent (agentId set) are shown as a nested lane under the main agent.
 */
@customElement("turn-trace-view")
export class TurnTraceView extends LitElement {
  @property({ attribute: false }) spans: TurnEventSpan[] = [];

  @state() private hovered: TurnEventSpan | null = null;
  @state() private hoverX = 0;
  @state() private hoverY = 0;

  createRenderRoot() {
    return this;
  }

  render() {
    if (!this.spans.length) return html``;
    const { totalAIU, lanes } = this.#buildLayout();
    const ticks = [0, 0.25, 0.5, 0.75, 1];

    return html`
      <div class="chart-wrap flex flex-col gap-2 relative">
        <div class="flex justify-between text-[0.7rem] text-muted">
          ${ticks.map((t) => html`<span>${formatAIU(totalAIU * t)}</span>`)}
        </div>
        <div class="flex flex-col gap-2.5">
          ${lanes.map(
            (lane) => html`
              <div class="${lane.depth ? "pl-4" : ""}">
                <div class="text-[0.75rem] text-muted mb-1">${lane.label}（${lane.items.length}件）</div>
                <div class="relative h-[22px] bg-[var(--toggle-track-bg)] rounded-md border border-border">
                  ${lane.items.map(
                    (p) => html`
                      <div
                        class="absolute top-0 h-full rounded-sm cursor-default"
                        style="left:calc(${p.leftPct}% + ${GAP_PX / 2}px);width:max(2px, calc(${p.widthPct}% - ${GAP_PX}px));background:${p.color}"
                        @mouseenter=${(e: MouseEvent) => this.#onHover(p.span, e)}
                        @mousemove=${(e: MouseEvent) => this.#onHover(p.span, e)}
                        @mouseleave=${() => this.#onLeave()}
                      ></div>
                    `,
                  )}
                </div>
              </div>
            `,
          )}
        </div>
        ${this.hovered ? this.#renderTooltip(this.hovered) : nothing}
      </div>
    `;
  }

  #onHover(span: TurnEventSpan, e: MouseEvent) {
    this.hovered = span;
    this.hoverX = e.clientX;
    this.hoverY = e.clientY;
  }

  #onLeave() {
    this.hovered = null;
  }

  #renderTooltip(span: TurnEventSpan) {
    return html`
      <div
        class="fixed z-[110] pointer-events-none text-[0.75rem] bg-card border border-border rounded-md px-2.5 py-1.5 [box-shadow:var(--shadow-md)] leading-relaxed"
        style="left:${this.hoverX + 12}px;top:${this.hoverY + 12}px"
      >
        <div class="font-semibold">${span.model}</div>
        <div>${formatAIU(span.aiu)} AIU</div>
        ${span.durationMs ? html`<div class="text-muted">${formatDurationMs(span.durationMs)}</div>` : nothing}
        ${span.initiator ? html`<div class="text-muted">${span.initiator}</div>` : nothing}
        ${span.finishReason ? html`<div class="text-muted">${span.finishReason}</div>` : nothing}
      </div>
    `;
  }

  #buildLayout(): { totalAIU: number; lanes: Lane[] } {
    const totalAIU = this.spans.reduce((sum, s) => sum + s.aiu, 0);

    const modelOrder: string[] = [];
    for (const s of this.spans) {
      if (!modelOrder.includes(s.model)) modelOrder.push(s.model);
    }
    const colorFor = (model: string) => PALETTE[modelOrder.indexOf(model) % PALETTE.length];

    let cumulative = 0;
    const positioned: PositionedSpan[] = this.spans.map((span) => {
      const leftPct = totalAIU > 0 ? (cumulative / totalAIU) * 100 : 0;
      const widthPct = totalAIU > 0 ? Math.max((span.aiu / totalAIU) * 100, MIN_WIDTH_PCT) : 100;
      cumulative += span.aiu;
      return { span, leftPct, widthPct, color: colorFor(span.model) };
    });

    const laneOrder: string[] = [];
    for (const p of positioned) {
      const key = p.span.agentId || "";
      if (!laneOrder.includes(key)) laneOrder.push(key);
    }
    // Stable sort: the main agent's lane (key "") always renders first,
    // sub-agent lanes keep their first-appearance order otherwise.
    laneOrder.sort((a, b) => (a === "" ? -1 : b === "" ? 1 : 0));

    const lanes: Lane[] = laneOrder.map((key) => ({
      key,
      label: key || "メインエージェント",
      depth: key ? 1 : 0,
      items: positioned.filter((p) => (p.span.agentId || "") === key),
    }));

    return { totalAIU, lanes };
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "turn-trace-view": TurnTraceView;
  }
}
