import { LitElement, html } from "lit";
import { customElement, property, query } from "lit/decorators.js";
import {
  Chart,
  BarController,
  BarElement,
  CategoryScale,
  LinearScale,
  Tooltip,
  type ChartConfiguration,
} from "chart.js";
import { formatAIU } from "../lib/format.js";
import { PALETTE } from "../lib/colors.js";

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Tooltip);

export interface BreakdownItem {
  label: string;
  value: number;
}

@customElement("breakdown-bar-chart")
export class BreakdownBarChart extends LitElement {
  @property({ attribute: false }) items: BreakdownItem[] = [];

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
        <canvas class="min-w-[280px] h-[260px]"></canvas>
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
    return {
      labels: this.items.map((i) => i.label),
      datasets: [
        {
          data: this.items.map((i) => i.value),
          backgroundColor: this.items.map((_, i) => PALETTE[i % PALETTE.length]),
        },
      ],
    };
  }

  #buildConfig(): ChartConfiguration<"bar"> {
    return {
      type: "bar",
      data: this.#buildData(),
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          y: { title: { display: true, text: "AIU" } },
        },
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: (ctx) => `${formatAIU(ctx.parsed.y ?? 0)} AIU`,
            },
          },
        },
      },
    };
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "breakdown-bar-chart": BreakdownBarChart;
  }
}
