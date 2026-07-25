import { LitElement, html } from "lit";
import { customElement, property, query } from "lit/decorators.js";
import { Chart, PieController, ArcElement, Legend, Tooltip, type ChartConfiguration } from "chart.js";
import { formatAIU } from "../lib/format.js";
import { t } from "../lib/i18n.js";
import { PALETTE } from "../lib/colors.js";

Chart.register(PieController, ArcElement, Legend, Tooltip);

export interface BreakdownItem {
  label: string;
  value: number;
}

@customElement("breakdown-pie-chart")
export class BreakdownPieChart extends LitElement {
  @property({ attribute: false }) items: BreakdownItem[] = [];

  @query("canvas") canvasEl!: HTMLCanvasElement;
  private chart?: Chart<"pie">;

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

  #buildConfig(): ChartConfiguration<"pie"> {
    return {
      type: "pie",
      data: this.#buildData(),
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { position: "bottom" },
          tooltip: {
            callbacks: {
              label: (ctx) =>
                `${ctx.label}: ${formatAIU(typeof ctx.parsed === "number" ? ctx.parsed : 0)} ${t("unitAIU")}`,
            },
          },
        },
      },
    };
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "breakdown-pie-chart": BreakdownPieChart;
  }
}
