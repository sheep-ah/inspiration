/** 通用界面工具：元素构造、格式化、提示。 */

/** 创建元素并设置属性/子节点。 */
export function h<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, string> = {},
  children: (Node | string)[] = []
): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") el.className = v;
    else if (k === "text") el.textContent = v;
    else el.setAttribute(k, v);
  }
  for (const c of children) el.append(c);
  return el;
}

/** HTML 转义（用于拼接 innerHTML 时插入文本）。 */
export function esc(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function fmtSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "-";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

const PART_LABELS: Record<string, string> = {
  "word/document.xml": "正文/表格",
  "word/header1.xml": "页眉",
  "word/footer1.xml": "页脚",
  "word/footnotes.xml": "脚注",
  "word/endnotes.xml": "尾注",
  "word/comments.xml": "批注",
};

export function fmtPart(part: string): string {
  const known = PART_LABELS[part];
  if (known) return known;
  if (part.startsWith("word/header")) return "页眉";
  if (part.startsWith("word/footer")) return "页脚";
  return part;
}

let toastTimer: number | undefined;

/** 右下角轻提示。 */
export function toast(message: string, ms = 2600): void {
  const existing = document.querySelector(".toast");
  if (existing) existing.remove();
  const el = h("div", { class: "toast", text: message });
  document.body.append(el);
  if (toastTimer) window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => el.remove(), ms);
}

const openModals = new Set<() => void>();

/** 关闭所有已打开的模态框（页面切换时调用，避免残留）。 */
export function closeAllModals(): void {
  for (const close of [...openModals]) close();
}

/** 简单模态框：内容由调用方提供，返回关闭函数。 */
export function modal(title: string, body: Node, actions: Node[]): () => void {
  const mask = h("div", { class: "modal-mask" });
  const box = h("div", { class: "modal" });
  box.append(h("h3", { class: "modal-title", text: title }));
  box.append(body);
  const footer = h("div", { class: "modal-actions" }, actions);
  box.append(footer);
  mask.append(box);
  const close = () => {
    openModals.delete(close);
    mask.remove();
  };
  openModals.add(close);
  mask.addEventListener("click", (e) => {
    if (e.target === mask) close();
  });
  document.body.append(mask);
  return close;
}

/** 二次确认对话框。 */
export function confirmDialog(title: string, message: string, confirmText = "确认"): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false;
    const done = (v: boolean) => {
      if (settled) return;
      settled = true;
      close();
      resolve(v);
    };
    const body = h("div", {}, [h("p", { text: message, style: "white-space:pre-wrap" })]);
    const cancel = h("button", { class: "btn", text: "取消" }, []);
    cancel.addEventListener("click", () => done(false));
    const ok = h("button", { class: "btn danger", text: confirmText }, []);
    ok.addEventListener("click", () => done(true));
    const close = modal(title, body, [cancel, ok]);
  });
}

/** 状态提示条。 */
export function alertBox(kind: "error" | "success" | "warn" | "info", message: string): HTMLElement {
  return h("div", { class: `alert ${kind}`, text: message });
}

export function busyRow(text: string): HTMLElement {
  return h("div", { class: "busy" }, [h("span", { class: "spinner" }), h("span", { text })]);
}
