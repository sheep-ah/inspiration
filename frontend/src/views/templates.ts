import { api, errorMessage, type Template, type TemplateDetail } from "../api";
import { alertBox, busyRow, confirmDialog, fmtPart, fmtSize, fmtTime, h, modal, toast } from "../ui";

/** 模板管理页。返回清理函数。 */
export function renderTemplates(root: HTMLElement): () => void {
  let templates: Template[] = [];
  let query = "";
  let supportedFields: string[] = [];

  const page = h("div");
  page.append(
    h("h1", { class: "page-title", text: "模板管理" }),
    h("p", {
      class: "page-desc",
      text: "上传 DOCX 模板（也支持旧版 .doc，会自动转换成 .docx）、查看占位符、重命名或删除。模板文件会复制到应用数据目录，原始文件不会被修改。",
    })
  );

  const alertArea = h("div");
  page.append(alertArea);

  const toolbarCard = h("div", { class: "card" });
  const toolbar = h("div", { class: "toolbar" });
  const searchInput = h("input", { class: "search", type: "text", placeholder: "搜索模板名称…" }) as HTMLInputElement;
  const importBtn = h("button", { class: "btn primary", text: "上传模板" });
  const refreshBtn = h("button", { class: "btn", text: "刷新" });
  toolbar.append(searchInput, importBtn, refreshBtn);
  toolbarCard.append(toolbar);
  const listArea = h("div", { style: "margin-top:14px" });
  toolbarCard.append(listArea);
  page.append(toolbarCard);
  root.append(page);

  function renderList(): void {
    listArea.innerHTML = "";
    if (templates.length === 0) {
      listArea.append(h("div", { class: "empty", text: query === "" ? "还没有模板，点击右上角「上传模板」开始。" : "没有匹配的模板。" }));
      return;
    }
    const list = h("div", { class: "list" });
    for (const t of templates) {
      const count = t.placeholders.reduce((sum, p) => sum + p.count, 0);
      const row = h("div", { class: "list-row" });

      const main = h("div", { class: "list-main" });
      const nameRow = h("div", { class: "row" });
      nameRow.append(h("div", { class: "list-name", text: t.name }));
      if (t.sourceFormat === "doc") nameRow.append(h("span", { class: "badge", text: "来自 .doc" }));
      if (!t.fileExists) nameRow.append(h("span", { class: "badge warn", text: "文件缺失" }));
      else if (count === 0) nameRow.append(h("span", { class: "badge warn", text: "无占位符" }));
      else nameRow.append(h("span", { class: "badge ok", text: `${count} 个占位符` }));
      main.append(nameRow);
      main.append(
        h("div", {
          class: "list-meta",
          text: `${fmtSize(t.sizeBytes)}｜更新于 ${fmtTime(t.updatedAt)}`,
        })
      );
      row.append(main);

      const actions = h("div", { class: "row-actions" });
      const detailBtn = h("button", { class: "btn link", text: "详情" });
      detailBtn.addEventListener("click", () => {
        void showDetail(t.id);
      });
      const renameBtn = h("button", { class: "btn link", text: "重命名" });
      renameBtn.addEventListener("click", () => {
        void rename(t);
      });
      const deleteBtn = h("button", { class: "btn link", text: "删除" });
      deleteBtn.addEventListener("click", () => {
        void remove(t);
      });
      actions.append(detailBtn, renameBtn, deleteBtn);
      row.append(actions);
      list.append(row);
    }
    listArea.append(list);
  }

  async function refresh(): Promise<void> {
    try {
      templates = await api.listTemplates(query);
      renderList();
    } catch (e) {
      alertArea.innerHTML = "";
      alertArea.append(alertBox("error", `读取模板列表失败：${errorMessage(e)}`));
    }
  }

  function detailBody(detail: TemplateDetail): Node {
    const body = h("div");
    if (detail.warning) {
      body.append(alertBox("warn", detail.warning));
    }
    if (detail.problems && detail.problems.length > 0) {
      body.append(alertBox("error", `模板存在问题，无法用于生成：\n${detail.problems.join("\n")}`));
      const unknown = detail.placeholders.filter((p) => p.unknown).map((p) => p.name);
      if (unknown.length > 0) {
        body.append(
          alertBox(
            "info",
            `模板里出现了本版本不支持的占位符：${unknown.map((n) => `{{${n}}}`).join("、")}\n` +
              (supportedFields.length > 0
                ? `本版本支持的字段（共 ${supportedFields.length} 个）：\n${supportedFields.map((f) => `{{${f}}}`).join("、")}\n` +
                  "请把模板里的占位符改成上面这些名字后重新上传（若你刚升级过应用，请先完全退出并重新打开）。"
                : "请确认占位符写法是否正确。")
          )
        );
      }
    } else {
      body.append(alertBox("success", "模板校验通过，可用于生成合同。"));
    }
    if (detail.missing && detail.missing.length > 0) {
      body.append(alertBox("info", `模板未使用的字段（可选）：${detail.missing.join("、")}`));
    }
    const table = h("table", { class: "simple" });
    const thead = h("thead");
    const hr = h("tr");
    for (const t of ["占位符", "出现次数", "所在位置"]) hr.append(h("th", { text: t }));
    thead.append(hr);
    table.append(thead);
    const tbody = h("tbody");
    if (detail.placeholders.length === 0) {
      const tr = h("tr");
      const td = h("td", { colspan: "3", text: "未识别到占位符，请检查模板中是否使用了 {{字段名}} 形式。" });
      tr.append(td);
      tbody.append(tr);
    }
    for (const p of detail.placeholders) {
      const tr = h("tr");
      const nameCell = h("td");
      nameCell.append(h("span", { class: "mono", text: `{{${p.name}}}` }));
      if (p.unknown) nameCell.append(h("span", { class: "badge warn", text: "不支持", style: "margin-left:6px" }));
      tr.append(nameCell);
      tr.append(h("td", { text: String(p.count) }));
      tr.append(h("td", { text: p.parts.map(fmtPart).join("、") }));
      tbody.append(tr);
    }
    table.append(tbody);
    body.append(table);
    body.append(
      h("div", { class: "list-meta", style: "margin-top:10px", text: `文件：${detail.template.name}｜${fmtSize(detail.template.sizeBytes)}｜SHA256 ${detail.template.sha256.slice(0, 16)}…` })
    );
    if (detail.template.sourceFormat === "doc") {
      body.append(
        h("div", {
          class: "list-meta",
          style: "margin-top:4px",
          text: "原始 .doc 已保留在应用数据目录的 templates 子目录中，便于对照与重新导入。",
        })
      );
    }
    return body;
  }

  async function showDetail(id: string): Promise<void> {
    try {
      const detail = await api.templateDetail(id);
      const closeBtn = h("button", { class: "btn", text: "关闭" });
      const close = modal(`模板详情：${detail.template.name}`, detailBody(detail), [closeBtn]);
      closeBtn.addEventListener("click", () => close());
    } catch (e) {
      toast(errorMessage(e));
    }
  }

  async function rename(t: Template): Promise<void> {
    const input = h("input", { class: "search", type: "text", value: t.name, style: "width:100%" }) as HTMLInputElement;
    const cancel = h("button", { class: "btn", text: "取消" });
    const ok = h("button", { class: "btn primary", text: "保存" });
    const close = modal("重命名模板", h("div", {}, [input]), [cancel, ok]);
    cancel.addEventListener("click", () => close());
    ok.addEventListener("click", () => {
      const name = input.value.trim();
      if (name === "") {
        toast("模板名称不能为空");
        return;
      }
      api
        .renameTemplate(t.id, name)
        .then(() => {
          close();
          toast("已重命名");
          return refresh();
        })
        .catch((e) => toast(errorMessage(e)));
    });
    input.focus();
    input.select();
  }

  async function remove(t: Template): Promise<void> {
    const confirmed = await confirmDialog(
      "删除模板",
      `确定要删除模板「${t.name}」吗？\n\n删除后该模板将从列表中移除，但已经生成的合同文件不受影响。`,
      "删除"
    );
    if (!confirmed) return;
    try {
      await api.deleteTemplate(t.id);
      toast("模板已删除");
      await refresh();
    } catch (e) {
      toast(errorMessage(e));
    }
  }

  async function importTemplate(): Promise<void> {
    alertArea.innerHTML = "";
    try {
      const detail = await api.importTemplate();
      if (!detail) return; // 用户取消
      await refresh();
      const closeBtn = h("button", { class: "btn primary", text: "知道了" });
      const close = modal(`已导入：${detail.template.name}`, detailBody(detail), [closeBtn]);
      closeBtn.addEventListener("click", () => close());
    } catch (e) {
      alertArea.append(alertBox("error", `导入失败：${errorMessage(e)}`));
    }
  }

  searchInput.addEventListener("input", () => {
    query = searchInput.value.trim();
    void refresh();
  });
  refreshBtn.addEventListener("click", () => {
    void refresh();
  });
  importBtn.addEventListener("click", () => {
    void importTemplate();
  });

  listArea.append(busyRow("正在读取模板列表…"));
  void refresh();
  api
    .supportedFields()
    .then((fields) => {
      supportedFields = fields;
    })
    .catch(() => {
      supportedFields = [];
    });

  return () => {
    /* 本页没有需要解绑的全局监听 */
  };
}
