import { defineConfig, presetWind3 } from "unocss";

export default defineConfig({
  presets: [presetWind3()],
  content: {
    pipeline: {
      include: [/\.(vue|svelte|[jt]sx|mdx?|astro|elm|php|phtml|html)($|\?)/, "src/**/*.ts"],
    },
  },
  theme: {
    colors: {
      bg: "var(--bg)",
      "bg-subtle": "var(--bg-subtle)",
      fg: "var(--fg)",
      muted: "var(--muted)",
      border: "var(--border)",
      card: "var(--card)",
      accent: "var(--accent)",
      danger: "var(--danger)",
    },
  },
  shortcuts: {
    "toggle-group": "inline-flex bg-bg-subtle border border-border rounded-md p-0.5 gap-0.5",
    "toggle-btn": "bg-transparent text-muted border border-transparent rounded text-[0.85rem] px-3 py-1 cursor-pointer",
    "toggle-btn-active": "bg-card text-fg border-border font-semibold [box-shadow:var(--shadow)]",
    "group-box": "bg-card border border-border rounded-md p-4 [box-shadow:var(--shadow)]",
    "group-title": "text-[0.85rem] font-semibold text-muted mb-2.5",
    "card-label": "text-xs text-muted",
    "card-value": "text-[1.4rem] font-semibold mt-1",
    "card-value-small": "text-[0.95rem] font-semibold mt-1",
    "card-error": "text-[0.8rem] text-danger",
    "chart-wrap": "bg-card border border-border rounded-md p-4 [box-shadow:var(--shadow)] overflow-x-auto",
    "modal-backdrop": "fixed inset-0 bg-[rgba(27,31,36,0.5)] flex items-start justify-center py-12 px-4 z-[100]",
    modal: "bg-card border border-border rounded-md [box-shadow:0_8px_24px_rgba(140,149,159,0.3)] max-w-[640px] w-full max-h-[calc(100vh-96px)] flex flex-col",
    "modal-header": "flex items-center justify-between px-4 py-3.5 border-b border-border",
    "modal-close": "bg-transparent border-none text-muted text-xl cursor-pointer leading-none px-2 py-1",
    "modal-body": "p-4 overflow-y-auto text-[0.9rem]",
    "modal-section-title": "text-[0.85rem] font-semibold mt-4 mb-2",
    "checkpoint-item": "border border-border rounded-md px-3 py-2.5 mb-2",
    "page-title": "text-xl font-semibold m-0 mb-4 pb-3 border-b border-border",
  },
  preflights: [
    {
      getCSS: () => `
        :root {
          color-scheme: light dark;
          --bg: #ffffff;
          --bg-subtle: #f6f8fa;
          --fg: #1f2328;
          --muted: #59636e;
          --border: #d1d9e0;
          --card: #ffffff;
          --accent: #0969da;
          --danger: #d1242f;
          --shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
        }
        @media (prefers-color-scheme: dark) {
          :root {
            --bg: #0d1117;
            --bg-subtle: #161b22;
            --fg: #e6edf3;
            --muted: #8b949e;
            --border: #30363d;
            --card: #0d1117;
            --accent: #58a6ff;
            --danger: #f85149;
            --shadow: 0 0 transparent;
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
        usage-dashboard, usage-summary-cards, usage-chart, session-detail-modal, usage-toggle-group {
          display: block;
        }
      `,
    },
  ],
});
