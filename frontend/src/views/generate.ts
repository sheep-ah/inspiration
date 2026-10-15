import {
  api,
  errorMessage,
  type ConverterStatus,
  type FieldDef,
  type FieldError,
  type GenerateResult,
  type Template,
} from "../api";
import { alertBox, busyRow, h, toast } from "../ui";

/** 合同生成页。返回清理函数。 */
export function renderGenerate(root: HTMLElement): () => void {
  const cleanups: (() => void)[] = [];

  const fields: FieldDef[] = [];
  const values: Record<string, string> = {};
  const inputs = new Map<string, HTMLInputElement>();
  const errorSlots = new Map<string, HTMLElement>();
  let templates: Template[] = [];
  let selected: Template | null = null;
  let outputDir = "";
  let busy = false;
  let outputFormat: "docx" | "doc" = "docx";
  let keepDocxCopy = true;
  let converter: ConverterStatus = { available: false, kind: "", name: "", path: "", detail: "正在检测…", channels: [] };

  const page = h("div");
  const pageDesc = h("p", { class: "page-desc", text: "选择模板并填写基础字段，生成文件保存在你指定的位置。" });
  page.append(h("h1", { class: "page-title", text: "合同生成" }), pageDesc);

  const alertArea = h("div");
  page.append(alertArea);

  // ---------------- 模板选择 ----------------
  const comboCards = h("div", { class: "card" });
  comboCards.append(h("h2", { class: "card-title", text: "1. 选择模板" }));

  const combo = h("div", { class: "combo" });
  const comboInput = h("input", {
    class: "search",
    type: "text",
    placeholder: "点击选择模板，或输入关键字搜索",
    autocomplete: "off",
    style: "width:100%",
  }) as HTMLInputElement;
  const comboList = h("div", { class: "combo-list" });
  combo.append(comboInput, comboList);
  comboCards.append(combo);

  const selectedInfo = h("div", { class: "list-meta", style: "margin-top:8px" });
  comboCards.append(selectedInfo);
  page.append(comboCards);

  // ---------------- 表单 ----------------
  const formCard = h("div", { class: "card" });
  formCard.append(h("h2", { class: "card-title", text: "2. 填写合同信息" }));
  const formGrid = h("div", { class: "grid" });
  formCard.append(formGrid);
  const formHint = h("div", { class: "list-meta", style: "margin-top:10px" });
  formCard.append(formHint);
  page.append(formCard);

  // ---------------- 输出 ----------------
  const outputCard = h("div", { class: "card" });
  outputCard.append(h("h2", { class: "card-title", text: "3. 保存位置" }));
  const outputRow = h("div", { class: "row" });
  const outputPath = h("div", { class: "mono", style: "flex:1" });
  const pickDirBtn = h("button", { class: "btn", text: "更改位置" });
  outputRow.append(outputPath, pickDirBtn);
  outputCard.append(outputRow);

  const nameRow = h("div", { class: "row", style: "margin-top:10px" });
  const nameLabel = h("span", { class: "list-meta", text: "文件名（可选）：" });
  const nameInput = h("input", { class: "search", type: "text", placeholder: "留空则自动命名为 合同-公司名-日期", style: "flex:1" }) as HTMLInputElement;
  nameRow.append(nameLabel, nameInput);
  outputCard.append(nameRow);

  // 输出格式：docx 永远可用；doc 需要本机 LibreOffice
  const formatRow = h("div", { class: "row", style: "margin-top:12px" });
  formatRow.append(h("span", { class: "list-meta", text: "输出格式：" }));
  const radioDocx = h("input", { type: "radio", id: "fmt-docx", name: "output-format", value: "docx", checked: "checked" }) as HTMLInputElement;
  const radioDoc = h("input", { type: "radio", id: "fmt-doc", name: "output-format", value: "doc" }) as HTMLInputElement;
  const labelDocx = h("label", { for: "fmt-docx", text: "DOCX" }) as HTMLLabelElement;
  const labelDoc = h("label", { for: "fmt-doc", text: "DOC（Word 97-2003）" }) as HTMLLabelElement;
  const converterHint = h("span", { class: "list-meta" });
  formatRow.append(radioDocx, labelDocx, radioDoc, labelDoc, converterHint);
  outputCard.append(formatRow);

  // 保留 .docx 备份（.doc 存在版式差异风险时很有用）
  const backupRow = h("div", { class: "row", style: "margin-top:8px" });
  const backupCheck = h("input", { type: "checkbox", id: "keep-docx", checked: "checked" }) as HTMLInputElement;
  const backupLabel = h("label", { for: "keep-docx", text: "同时保留一份 .docx 备份（推荐）" }) as HTMLLabelElement;
  backupRow.append(backupCheck, backupLabel, h("span", { class: "list-meta", text: ".doc 用于交付，.docx 用于打印/存档/核对" }));
  outputCard.append(backupRow);
  backupCheck.addEventListener("change", () => {
    keepDocxCopy = backupCheck.checked;
  });

  radioDocx.addEventListener("change", () => {
    if (radioDocx.checked) outputFormat = "docx";
  });
  radioDoc.addEventListener("change", () => {
    if (radioDoc.checked) outputFormat = "doc";
  });

  function renderConverterStatus(): void {
    if (converter.available) {
      radioDoc.disabled = false;
      const weak = converter.kind === "libreoffice";
      converterHint.textContent = weak
        ? `将使用 ${converter.name} 转换（文本框/图文框位置可能与原稿不同，建议保留 .docx 备份）`
        : `将使用 ${converter.name} 转换，保真度最高；全程本机处理，不会上传`;
      converterHint.title = converter.channels.map((c) => `${c.name}：${c.detail}`).join("\n");
    } else {
      radioDoc.disabled = true;
      radioDocx.checked = true;
      outputFormat = "docx";
      converterHint.textContent = "本机未检测到 Word / WPS / LibreOffice，暂不能输出 .doc";
      converterHint.title = converter.detail;
    }
  }
  page.append(outputCard);

  // ---------------- 操作 ----------------
  const actionCard = h("div", { class: "card" });
  const generateBtn = h("button", { class: "btn primary", text: "生成合同" });
  const actionRow = h("div", { class: "row" });
  const statusSlot = h("div", { class: "list-meta" });
  actionRow.append(generateBtn, statusSlot);
  actionCard.append(actionRow);
  const resultSlot = h("div");
  actionCard.append(resultSlot);
  page.append(actionCard);

  root.append(page);

  // ---------------- 行为 ----------------
  function refreshOutputPath(): void {
    outputPath.textContent = outputDir === "" ? "（未设置）" : outputDir;
  }

  function closeList(): void {
    comboList.classList.remove("open");
  }

  function openList(): void {
    comboList.classList.add("open");
  }

  function renderList(): void {
    const q = comboInput.value.trim().toLowerCase();
    const matched = templates.filter((t) => q === "" || t.name.toLowerCase().includes(q));
    comboList.innerHTML = "";
    if (matched.length === 0) {
      comboList.append(h("div", { class: "combo-empty", text: templates.length === 0 ? "还没有模板，请先到「模板管理」上传。" : "没有匹配的模板" }));
      return;
    }
    for (const t of matched) {
      const count = t.placeholders.reduce((sum, p) => sum + p.count, 0);
      const item = h("div", { class: "combo-item" });
      item.append(h("span", { text: t.name }));
      item.append(h("span", { class: "meta", text: `占位符 ${count}${t.fileExists ? "" : "（文件缺失）"}` }));
      item.addEventListener("click", () => {
        selected = t;
        comboInput.value = t.name;
        closeList();
        renderSelected();
      });
      comboList.append(item);
    }
  }

  function renderSelected(): void {
    if (!selected) {
      selectedInfo.textContent = "尚未选择模板";
      return;
    }
    const names = Object.keys(selected.fieldUsage);
    selectedInfo.textContent =
      `已选择：${selected.name}｜使用字段 ${names.length} 个（${names.join("、") || "无"}）` +
      (selected.fileExists ? "" : "｜⚠ 模板文件缺失");
  }

  function renderFields(): void {
    formGrid.innerHTML = "";
    inputs.clear();
    errorSlots.clear();
    for (const def of fields) {
      const wide = def.maxRunes > 100 ? " wide" : "";
      const field = h("div", { class: `field${wide}` });
      field.append(h("label", { text: `${def.label}（必填）` }));
      const input = h("input", { type: "text", placeholder: def.hint, value: values[def.key] ?? "", autocomplete: "off" }) as HTMLInputElement;
      input.addEventListener("input", () => {
        values[def.key] = input.value;
        clearFieldError(def.key);
      });
      input.addEventListener("blur", () => {
        void validateField(def.key);
      });
      const err = h("div", { class: "error" });
      field.append(input, err);
      inputs.set(def.key, input);
      errorSlots.set(def.key, err);
      formGrid.append(field);
    }
    formHint.textContent = "生成时只替换模板中出现的占位符；模板未使用的字段不影响生成结果（当前共 " + fields.length + " 个字段）。";
  }

  function clearFieldError(key: string): void {
    const slot = errorSlots.get(key);
    if (slot) slot.textContent = "";
    const input = inputs.get(key);
    if (input) input.parentElement?.classList.remove("invalid");
  }

  function showFieldErrors(errs: FieldError[]): void {
    for (const e of errs) {
      const slot = errorSlots.get(e.key);
      if (slot) slot.textContent = e.message;
      const input = inputs.get(e.key);
      if (input) input.parentElement?.classList.add("invalid");
    }
  }

  async function validateField(key: string): Promise<void> {
    try {
      const errs = await api.validateForm(values);
      const hit = errs.find((e) => e.key === key);
      if (hit) {
        const slot = errorSlots.get(key);
        if (slot) slot.textContent = hit.message;
        inputs.get(key)?.parentElement?.classList.add("invalid");
      } else {
        clearFieldError(key);
      }
    } catch {
      /* 校验失败不阻塞输入 */
    }
  }

  function setBusy(next: boolean): void {
    busy = next;
    generateBtn.toggleAttribute("disabled", busy);
    statusSlot.innerHTML = "";
    if (busy) statusSlot.append(busyRow("正在生成，请稍候…"));
  }

  function showResult(res: GenerateResult): void {
    resultSlot.innerHTML = "";
    const box = h("div", { class: "alert success", style: "margin-top:12px" });
    box.append(
      h("div", {
        text: `生成成功（${res.format === "doc" ? "DOC" : "DOCX"}${res.converterName ? " · " + res.converterName : ""}）：${res.fileName}`,
      })
    );
    box.append(h("div", { class: "mono", text: res.filePath }));
    const actions = h("div", { class: "row", style: "margin-top:8px" });
    const openBtn = h("button", { class: "btn primary", text: "打开文件" });
    openBtn.addEventListener("click", () => {
      api.openFile(res.filePath).catch((e) => toast(errorMessage(e)));
    });
    const revealBtn = h("button", { class: "btn", text: "打开所在文件夹" });
    revealBtn.addEventListener("click", () => {
      api.revealInFolder(res.filePath).catch((e) => toast(errorMessage(e)));
    });
    actions.append(openBtn, revealBtn);
    box.append(actions);
    resultSlot.append(box);
    if (res.backupPath) {
      const backupBox = h("div", { class: "alert info", style: "margin-top:8px" });
      backupBox.append(h("div", { text: "已同时保留 .docx 备份（版式保真，可用于打印/存档）：" }));
      backupBox.append(h("div", { class: "mono", text: res.backupPath }));
      const openBackup = h("button", { class: "btn", text: "打开备份" });
      openBackup.addEventListener("click", () => {
        api.openFile(res.backupPath).catch((e) => toast(errorMessage(e)));
      });
      backupBox.append(h("div", { class: "row", style: "margin-top:6px" }, [openBackup]));
      resultSlot.append(backupBox);
    }
    if (res.warning) {
      resultSlot.append(alertBox("warn", res.warning));
    }
  }

  async function generate(): Promise<void> {
    if (busy) return;
    alertArea.innerHTML = "";
    resultSlot.innerHTML = "";
    if (!selected) {
      alertArea.append(alertBox("error", "请先选择模板。"));
      return;
    }
    const errs = await api.validateForm(values).catch(() => [] as FieldError[]);
    if (errs.length > 0) {
      showFieldErrors(errs);
      alertArea.append(alertBox("error", `请先修正 ${errs.length} 处填写问题。`));
      return;
    }
    setBusy(true);
    try {
      const res = await api.generateContract({
        templateId: selected.id,
        values,
        outputDir,
        fileName: nameInput.value.trim(),
        outputFormat,
        keepDocxCopy,
      });
      showResult(res);
      toast("合同已生成");
    } catch (e) {
      alertArea.append(alertBox("error", errorMessage(e)));
    } finally {
      setBusy(false);
    }
  }

  generateBtn.addEventListener("click", () => {
    void generate();
  });
  pickDirBtn.addEventListener("click", () => {
    api
      .pickOutputDir(outputDir)
      .then((dir) => {
        if (dir) {
          outputDir = dir;
          refreshOutputPath();
        }
      })
      .catch((e) => toast(errorMessage(e)));
  });
  comboInput.addEventListener("focus", () => {
    renderList();
    openList();
  });
  comboInput.addEventListener("input", () => {
    renderList();
    openList();
  });
  const onDocumentClick = (e: MouseEvent) => {
    if (!combo.contains(e.target as Node)) closeList();
  };
  document.addEventListener("click", onDocumentClick);
  cleanups.push(() => document.removeEventListener("click", onDocumentClick));

  // ---------------- 初始化 ----------------
  void (async () => {
    try {
      const [defs, list, dir, conv] = await Promise.all([
        api.fieldDefs(),
        api.listTemplates(""),
        api.defaultOutputDir(),
        api.converterStatus(),
      ]);
      fields.push(...defs);
      pageDesc.textContent = `选择模板并填写 ${fields.length} 个基础字段，生成文件保存在你指定的位置。`;
      templates = list;
      outputDir = dir;
      converter = conv;
      renderConverterStatus();
      renderFields();
      refreshOutputPath();
      renderList();
      renderSelected();
      if (templates.length === 1 && templates[0]) {
        selected = templates[0];
        comboInput.value = selected.name;
        renderSelected();
      }
    } catch (e) {
      alertArea.append(alertBox("error", `加载失败：${errorMessage(e)}`));
    }
  })();

  return () => {
    for (const fn of cleanups) fn();
  };
}
