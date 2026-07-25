import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";
import type { TurnEventSpan } from "../lib/api.js";
import { formatAIU, formatDurationMs } from "../lib/format.js";
import { PALETTE } from "../lib/colors.js";

// Minimum visible width for a span whose AIU share of the turn would
// otherwise round to an invisible sliver.
const MIN_WIDTH_PCT = 1.5;

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

  createRenderRoot() {
    return this;
  }

  render() {
    if (!this.spans.length) return html``;
    const { totalAIU, lanes } = this.#buildLayout();
    const ticks = [0, 0.25, 0.5, 0.75, 1];

    return html`
      <div class="chart-wrap flex flex-col gap-2">
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
                        class="absolute top-0 h-full rounded-sm"
                        style="left:${p.leftPct}%;width:${p.widthPct}%;background:${p.color}"
                        title=${this.#tooltip(p.span)}
                      ></div>
                    `,
                  )}
                </div>
              </div>
            `,
          )}
        </div>
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

  #tooltip(span: TurnEventSpan): string {
    const parts = [`${span.model}`, `${formatAIU(span.aiu)} AIU`];
    if (span.durationMs) parts.push(formatDurationMs(span.durationMs));
    if (span.initiator) parts.push(span.initiator);
    if (span.finishReason) parts.push(span.finishReason);
    return parts.join(" ・ ");
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "turn-trace-view": TurnTraceView;
  }
}
