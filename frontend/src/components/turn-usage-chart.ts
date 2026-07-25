import { LitElement, html } from "lit";
import { customElement, property, query } from "lit/decorators.js";
import {
  Chart,
  BarController,
  BarElement,
  CategoryScale,
  LinearScale,
  Legend,
  Tooltip,
  type ChartConfiguration,
} from "chart.js";
import type { SessionTurnUsage } from "../lib/api.js";
import { formatAIU } from "../lib/format.js";
import { PALETTE } from "../lib/colors.js";

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);

@customElement("turn-usage-chart")
export class TurnUsageChart extends LitElement {
  @property({ attribute: false }) turns: SessionTurnUsage[] = [];

  @query("canvas") canvasEl!: HTMLCanvasElement;
  private chart?: Chart<"bar">;

  createRenderRoot() {
    return this;
  }

  disconnectedCallback() {
    this.chart?.destroy();
    this.chart = undefined;
    super.disconnectedCallback();
  }

  render() {
    return html`
      <div class="chart-wrap">
        <canvas class="min-w-[480px] h-[300px]"></canvas>
      </div>
    `;
  }

  updated() {
    if (this.chart) {
      this.chart.data = this.#buildData();
      this.chart.update();
    } else {
      this.chart = new Chart(this.canvasEl, this.#buildConfig());
    }
  }

  #buildData() {
    const labels = this.turns.map((t) => (t.turnIndex >= 0 ? `#${t.turnIndex}` : "未割当"));
    const models: string[] = [];
    for (const t of this.turns) {
      for (const m of t.byModel) {
        if (!models.includes(m.model)) models.push(m.model);
      }
    }
    const datasets = models.map((model, i) => ({
      label: model,
      data: this.turns.map((t) => t.byModel.find((m) => m.model === model)?.aiu ?? 0),
      backgroundColor: PALETTE[i % PALETTE.length],
    }));
    return { labels, datasets };
  }

  #buildConfig(): ChartConfiguration<"bar"> {
    return {
      type: "bar",
      data: this.#buildData(),
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: { stacked: true },
          y: { stacked: true, title: { display: true, text: "AIU" } },
        },
        plugins: {
          legend: { position: "bottom" },
          tooltip: {
            callbacks: {
              label: (ctx) => `${ctx.dataset.label}: ${formatAIU(ctx.parsed.y ?? 0)} AIU`,
            },
          },
        },
      },
    };
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "turn-usage-chart": TurnUsageChart;
  }
}
