import "./style.css";

import { renderAbout } from "./views/about";
import { renderGenerate } from "./views/generate";
import { renderTemplates } from "./views/templates";
import { closeAllModals, h } from "./ui";

type ViewName = "generate" | "templates" | "about";

interface ViewDef {
  name: ViewName;
  label: string;
  render: (root: HTMLElement) => () => void;
}

const views: ViewDef[] = [
  { name: "generate", label: "合同生成", render: renderGenerate },
  { name: "templates", label: "模板管理", render: renderTemplates },
  { name: "about", label: "关于", render: renderAbout },
];

const app = document.querySelector<HTMLDivElement>("#app");
if (!app) {
  throw new Error("缺少 #app 容器");
}

const shell = h("div", { class: "shell" });
const sidebar = h("aside", { class: "sidebar" });
const brand = h("div", { class: "brand" });
brand.append(h("div", { class: "brand-mark", text: "灵" }));
const brandText = h("div", {});
brandText.append(h("div", { class: "brand-name", text: "灵感" }));
brandText.append(h("div", { class: "brand-sub", text: "本地合同生成" }));
brand.append(brandText);
sidebar.append(brand);

const nav = h("nav", { class: "nav" });
const navButtons = new Map<ViewName, HTMLButtonElement>();
for (const v of views) {
  const btn = h("button", { text: v.label });
  btn.addEventListener("click", () => {
    window.location.hash = `#/${v.name}`;
  });
  navButtons.set(v.name, btn);
  nav.append(btn);
}
sidebar.append(nav);

const footer = h("div", { class: "sidebar-footer" });
footer.append(h("div", { text: "数据仅在本机处理" }));
sidebar.append(footer);

const main = h("main", { class: "main" });
shell.append(sidebar, main);
app.append(shell);

let dispose: (() => void) | null = null;
let current: ViewName | null = null;

function routeName(): ViewName {
  const raw = window.location.hash.replace(/^#\/?/, "");
  const found = views.find((v) => v.name === raw);
  return found ? found.name : "generate";
}

function route(): void {
  const name = routeName();
  if (name === current) return;
  current = name;
  if (dispose) {
    dispose();
    dispose = null;
  }
  closeAllModals();
  for (const [key, btn] of navButtons) {
    btn.classList.toggle("active", key === name);
  }
  main.innerHTML = "";
  const view = views.find((v) => v.name === name);
  if (view) dispose = view.render(main);
}

window.addEventListener("hashchange", route);
if (window.location.hash === "") {
  window.location.hash = "#/generate";
} else {
  route();
}
route();
