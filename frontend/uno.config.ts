import path from "node:path";
import { defineConfig, presetWind3 } from "unocss";

const templatesDir = path.resolve(import.meta.dirname, "../internal/server/templates");

export default defineConfig({
  presets: [presetWind3()],
  content: {
    // also gates content.filesystem below — a file the glob finds still
    // gets dropped here unless its extension is listed.
    pipeline: {
      include: [/\.(vue|svelte|[jt]sx|mdx?|astro|elm|php|phtml|templ|html)($|\?)/, "src/**/*.ts"],
    },
    // the templ shell lives outside Vite's module graph, so it's never seen
    // by the pipeline scan above — read it straight off disk instead.
    filesystem: [`${templatesDir}/**/*.templ`],
  },
  theme: {
    colors: {
      bg: "var(--bg)",
      fg: "var(--fg)",
      muted: "var(--muted)",
      border: "var(--border)",
      card: "var(--card)",
      accent: "var(--accent)",
      danger: "var(--danger)",
      hover: "var(--hover)",
    },
  },
  shortcuts: {
    "toggle-group": "inline-flex bg-[var(--toggle-track-bg)] border border-border rounded-md p-0.5 gap-0.5",
    "toggle-btn":
      "bg-transparent text-fg border border-transparent rounded-[5px] text-[0.85rem] px-3 py-1 cursor-pointer focus-visible:[outline:2px_solid_var(--focus-ring)] focus-visible:[outline-offset:-2px]",
    // Only the unselected/idle buttons react to hover/active — the selected
    // one keeps its knob background regardless, same as Primer's
    // SegmentedControl (`.Button:not([aria-pressed='true'], [aria-disabled='true'])`).
    "toggle-btn-idle": "hover:bg-[var(--toggle-track-hover)] active:bg-[var(--toggle-track-active)]",
    "toggle-btn-active": "bg-[var(--toggle-knob-bg)] text-fg border-border font-semibold",
    "group-box": "bg-card border border-border rounded-md p-4 [box-shadow:var(--shadow-md)]",
    "group-title": "text-[0.85rem] font-semibold text-muted mb-2.5",
    "card-label": "text-[0.75rem] text-muted",
    "card-value": "text-[1.4rem] font-semibold mt-1",
    "card-value-small": "text-[0.95rem] font-semibold mt-1",
    "card-error": "text-[0.8rem] text-danger",
    "chart-wrap": "bg-card border border-border rounded-md p-4 [box-shadow:var(--shadow-md)] overflow-x-auto",
    // A large series count (esp. per-session labels) must scroll within its
    // own strip rather than shrink the plot area above it — see usage-chart.ts.
    "chart-legend": "flex flex-wrap gap-x-3 gap-y-1 max-h-28 overflow-y-auto pt-2 border-t border-border",
    "chart-legend-item":
      "inline-flex items-center gap-1.5 max-w-[220px] bg-transparent border-none rounded-[4px] px-1.5 py-0.5 text-[0.8rem] text-fg cursor-pointer hover:bg-hover focus-visible:[outline:2px_solid_var(--focus-ring)] focus-visible:[outline-offset:-2px]",
    "chart-legend-item-hidden": "opacity-50 line-through",
    "chart-legend-swatch": "w-2.5 h-2.5 rounded-[2px] shrink-0",
    "chart-legend-label": "overflow-hidden text-ellipsis whitespace-nowrap",
    "modal-backdrop": "fixed inset-0 bg-[rgba(27,31,36,0.5)] flex items-start justify-center py-12 px-4 z-[100]",
    modal: "bg-card border border-border rounded-md [box-shadow:0_8px_24px_rgba(140,149,159,0.3)] max-w-[640px] w-full max-h-[calc(100vh-96px)] flex flex-col",
    "modal-header": "flex items-center justify-between px-4 py-3.5 border-b border-border",
    "modal-close":
      "bg-transparent border-none text-muted text-[1.2rem] cursor-pointer leading-none px-2 py-1 rounded-md hover:bg-hover focus-visible:[outline:2px_solid_var(--focus-ring)] focus-visible:[outline-offset:-2px]",
    "modal-body": "p-4 overflow-y-auto",
    "modal-section-title": "text-[0.85rem] font-semibold mt-4 mb-2",
    "turn-item": "border border-border rounded-md px-3 py-2.5 mb-2",
    "page-title": "text-[1.25rem] font-semibold m-0 mb-4 pb-3 border-b border-border",
  },
  preflights: [
    {
      // Values are GitHub's own Primer design tokens (@primer/primitives
      // functional/themes/{light,dark}.css) — fgColor-*/bgColor-*/
      // borderColor-*/shadow-resting-*/control-*/focus-outline-color —
      // copied here rather than importing the full multi-theme stylesheet,
      // since this app switches on prefers-color-scheme, not Primer's
      // data-color-mode attribute.
      //
      // --bg is the page canvas (bgColor-muted) and --card is the raised
      // surface (bgColor-default) that sits on it — on github.com itself,
      // page chrome and card/box content are rarely the same color; using
      // one shade for both (as the very first pass here did) is what made
      // the cards look flat/washed out against the page.
      getCSS: () => `
        :root {
          color-scheme: light dark;
          --bg: #f6f8fa;
          --fg: #1f2328;
          --muted: #59636e;
          --border: #d1d9e0;
          --card: #ffffff;
          --accent: #0969da;
          --danger: #d1242f;
          --shadow: 0 1px 1px 0 rgba(31, 35, 40, 0.04), 0 1px 2px 0 rgba(31, 35, 40, 0.03);
          --shadow-md: 0 1px 1px 0 rgba(37, 41, 46, 0.1), 0 3px 6px 0 rgba(37, 41, 46, 0.12);
          --hover: rgba(129, 139, 152, 0.1);
          --focus-ring: #0969da;
          /* controlTrack and controlKnob tokens — the segmented-control-
             specific pair, distinct from the generic control- tokens used
             for ordinary form controls. */
          --toggle-track-bg: #e6eaef;
          --toggle-track-hover: #e0e6eb;
          --toggle-track-active: #dae0e7;
          --toggle-knob-bg: #ffffff;
        }
        @media (prefers-color-scheme: dark) {
          :root {
            --bg: #151b23;
            --fg: #f0f6fc;
            --muted: #9198a1;
            --border: #3d444d;
            --card: #0d1117;
            --accent: #4493f8;
            --danger: #f85149;
            --shadow: 0 1px 1px 0 rgba(1, 4, 9, 0.6), 0 1px 3px 0 rgba(1, 4, 9, 0.6);
            --shadow-md: 0 1px 1px 0 rgba(1, 4, 9, 0.4), 0 3px 6px 0 rgba(1, 4, 9, 0.8);
            --hover: rgba(101, 108, 118, 0.2);
            --focus-ring: #1f6feb;
            --toggle-track-bg: #010409;
            --toggle-track-hover: #2a313c;
            --toggle-track-active: #2f3742;
            --toggle-knob-bg: #262c36;
          }
        }
        * { box-sizing: border-box; }
        body {
          margin: 0;
          font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans JP", Helvetica, Arial, sans-serif;
          background: var(--bg);
          color: var(--fg);
          padding: 24px;
        }
        usage-dashboard, usage-summary-cards, usage-chart, session-detail-modal, usage-toggle-group,
        breakdown-pie-chart, turn-usage-chart {
          display: block;
        }
      `,
    },
  ],
});
