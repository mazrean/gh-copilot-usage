import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";

export interface ToggleOption {
  value: string;
  label: string;
}

@customElement("usage-toggle-group")
export class UsageToggleGroup extends LitElement {
  @property({ attribute: false }) options: ToggleOption[] = [];
  @property({ type: String }) value = "";

  createRenderRoot() {
    return this;
  }

  #select(value: string) {
    if (value === this.value) return;
    this.value = value;
    this.dispatchEvent(new CustomEvent("change", { detail: { value }, bubbles: true, composed: true }));
  }

  render() {
    return html`
      <div class="toggle-group">
        ${this.options.map(
          (opt) => html`
            <button
              type="button"
              class="toggle-btn ${opt.value === this.value ? "toggle-btn-active" : "toggle-btn-idle"}"
              aria-pressed=${opt.value === this.value}
              @click=${() => this.#select(opt.value)}
            >
              ${opt.label}
            </button>
          `,
        )}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "usage-toggle-group": UsageToggleGroup;
  }
}
