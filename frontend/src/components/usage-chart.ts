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
import type { Usage } from "../lib/api.js";
import { formatAIU } from "../lib/format.js";

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);

const PALETTE = [
  "#2563eb",
  "#e07a5f",
  "#3a86ff",
  "#ffb703",
  "#8ecae6",
  "#c1121f",
  "#588157",
  "#9d4edd",
  "#f4a261",
  "#219ebc",
];

interface ChartDataset {
  label: string;
  sessionId: string | null;
  data: number[];
  backgroundColor: string;
}

@customElement("usage-chart")
export class UsageChart extends LitElement {
  @property({ attribute: false }) usage: Usage | null = null;

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
        <canvas class="min-w-[480px] h-[420px]"></canvas>
      </div>
    `;
  }

  updated() {
    if (!this.usage) return;
    if (this.chart) {
      this.chart.data = this.#buildData();
      this.chart.update();
    } else {
      this.chart = new Chart(this.canvasEl, this.#buildConfig());
    }
  }

  #buildData() {
    const usage = this.usage!;
    const isSession = usage.dimension === "session";
    const datasets: ChartDataset[] = usage.series.map((s, i) => ({
      label: s.label || s.key,
      sessionId: isSession && s.key !== "unknown" ? s.key : null,
      data: s.values,
      backgroundColor: PALETTE[i % PALETTE.length],
    }));
    return { labels: usage.buckets, datasets };
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
        onHover: (event, elements) => {
          const el = elements[0];
          const hit = el && (this.chart!.data.datasets[el.datasetIndex] as unknown as ChartDataset);
          const target = event.native?.target as HTMLElement | null;
          if (target) target.style.cursor = hit?.sessionId ? "pointer" : "default";
        },
        onClick: (_event, elements) => {
          const el = elements[0];
          if (!el) return;
          const dataset = this.chart!.data.datasets[el.datasetIndex] as unknown as ChartDataset;
          if (dataset.sessionId) {
            this.dispatchEvent(
              new CustomEvent("session-click", {
                detail: { sessionId: dataset.sessionId },
                bubbles: true,
                composed: true,
              }),
            );
          }
        },
      },
    };
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "usage-chart": UsageChart;
  }
}
