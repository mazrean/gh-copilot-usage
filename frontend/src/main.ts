import "uno.css";
import { getLocale, t } from "./lib/i18n.js";
import "./components/usage-dashboard.js";

document.documentElement.lang = getLocale();
document.title = t("docTitle");
const titleEl = document.querySelector(".page-title");
if (titleEl) titleEl.textContent = t("appTitle");
