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
  type Plugin,
} from "chart.js";
import type { Usage } from "../lib/api.js";
import { formatAIU, formatBucketLabel } from "../lib/format.js";
import { t } from "../lib/i18n.js";
import { PALETTE } from "../lib/colors.js";

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);

interface ChartDataset {
  label: string;
  sessionId: string | null;
  modelKey: string | null;
  data: number[];
  backgroundColor: string;
}

@customElement("usage-chart")
export class UsageChart extends LitElement {
  @property({ attribute: false }) usage: Usage | null = null;

  @query("canvas") canvasEl!: HTMLCanvasElement;
  @query(".chart-legend") legendEl!: HTMLDivElement;
  private chart?: Chart<"bar">;

  // Chart.js draws its built-in legend inside the fixed-height canvas, so a
  // large session/model count pushes the plot area to nothing. Rendering the
  // legend as its own scrollable HTML block (official "HTML Legend" recipe)
  // keeps the plot area a stable size regardless of label count.
  #htmlLegendPlugin: Plugin<"bar"> = {
    id: "htmlLegend",
    afterUpdate: (chart) => {
      const container = this.legendEl;
      if (!container) return;
      container.replaceChildren();
      const items = chart.options.plugins?.legend?.labels?.generateLabels?.(chart) ?? [];
      for (const item of items) {
        const button = document.createElement("button");
        button.type = "button";
        // Written as two full literal strings (not a template-interpolated
        // suffix) so UnoCSS's static class scanner can see "chart-legend-item"
        // as a whole token and actually generate its rules.
        button.className = item.hidden ? "chart-legend-item chart-legend-item-hidden" : "chart-legend-item";
        button.title = item.text;
        button.onclick = () => {
          if (item.datasetIndex == null) return;
          chart.setDatasetVisibility(item.datasetIndex, !chart.isDatasetVisible(item.datasetIndex));
          chart.update();
        };

        const swatch = document.createElement("span");
        swatch.className = "chart-legend-swatch";
        swatch.style.backgroundColor = String(item.fillStyle ?? item.strokeStyle ?? "");

        const label = document.createElement("span");
        label.className = "chart-legend-label";
        label.textContent = item.text;

        button.append(swatch, label);
        container.append(button);
      }
    },
  };

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
        <div class="flex flex-row items-stretch gap-4 h-[420px]">
          <div class="relative min-w-[480px] flex-1">
            <canvas></canvas>
          </div>
          <div class="chart-legend"></div>
        </div>
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
    const isModel = usage.dimension === "model";
    const datasets: ChartDataset[] = usage.series.map((s, i) => ({
      label: s.label || s.key,
      sessionId: isSession && s.key !== "unknown" ? s.key : null,
      modelKey: isModel && s.key !== "unknown" ? s.key : null,
      data: s.values,
      backgroundColor: PALETTE[i % PALETTE.length],
    }));
    const labels = usage.buckets.map((b) => formatBucketLabel(b, usage.granularity));
    return { labels, datasets };
  }

  #buildConfig(): ChartConfiguration<"bar"> {
    return {
      type: "bar",
      data: this.#buildData(),
      plugins: [this.#htmlLegendPlugin],
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: { stacked: true },
          y: { stacked: true, title: { display: true, text: t("unitAIU") } },
        },
        plugins: {
          // Rendered via #htmlLegendPlugin instead so a large label count
          // scrolls in its own block rather than shrinking the plot area.
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: (ctx) => `${ctx.dataset.label}: ${formatAIU(ctx.parsed.y ?? 0)} ${t("unitAIU")}`,
            },
          },
        },
        onHover: (event, elements) => {
          const el = elements[0];
          const hit = el && (this.chart!.data.datasets[el.datasetIndex] as unknown as ChartDataset);
          const target = event.native?.target as HTMLElement | null;
          if (target) target.style.cursor = hit?.sessionId || hit?.modelKey ? "pointer" : "default";
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
          } else if (dataset.modelKey) {
            this.dispatchEvent(
              new CustomEvent("model-click", {
                detail: { model: dataset.modelKey },
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
