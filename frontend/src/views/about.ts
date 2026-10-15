import {
  api,
  errorMessage,
  type ContractRecord,
  type UpdateInfo,
  type UpdateProgress,
  type VersionInfo,
} from "../api";
import { alertBox, fmtSize, fmtTime, h, toast } from "../ui";

/** 关于页：版本信息、应用更新、数据目录、隐私说明与生成记录。 */
export function renderAbout(root: HTMLElement): () => void {
  const page = h("div");
  page.append(
    h("h1", { class: "page-title", text: "关于" }),
    h("p", { class: "page-desc", text: "版本信息、应用更新、数据存放位置与本地处理说明。" })
  );

  const alertArea = h("div");
  page.append(alertArea);

  // ---------------- 版本与更新 ----------------
  const versionCard = h("div", { class: "card" });
  versionCard.append(h("h2", { class: "card-title", text: "版本" }));
  const versionBody = h("div", { class: "mono", text: "读取中…" });
  versionBody.style.whiteSpace = "pre-wrap";
  versionCard.append(versionBody);

  const updateRow = h("div", { class: "row", style: "margin-top:12px" });
  const checkBtn = h("button", { class: "btn primary", text: "检查更新" });
  const updateStatus = h("span", { class: "list-meta" });
  updateRow.append(checkBtn, updateStatus);

  const autoCheckBox = h("input", { type: "checkbox", id: "auto-check-update" }) as HTMLInputElement;
  const autoCheckLabel = h("label", { for: "auto-check-update", text: "启动时自动检查更新" }) as HTMLLabelElement;
  updateRow.append(h("span", { style: "margin-left:8px" }), autoCheckBox, autoCheckLabel);
  versionCard.append(updateRow);

  // 更新详情容器（有新版本时填充）
  const updateArea = h("div");
  versionCard.append(updateArea);

  // 进度条
  const progressWrap = h("div", { style: "margin-top:10px; display:none" });
  const progressBar = h("div", {
    style: "height:8px; background:#e5e7eb; border-radius:999px; overflow:hidden",
  });
  const progressInner = h("div", { style: "height:100%; width:0%; background:#2563eb; transition:width .2s" });
  progressBar.append(progressInner);
  const progressText = h("div", { class: "list-meta", style: "margin-top:6px", text: "" });
  progressWrap.append(progressBar, progressText);
  versionCard.append(progressWrap);

  page.append(versionCard);

  // ---------------- 数据目录 ----------------
  const dataCard = h("div", { class: "card" });
  dataCard.append(h("h2", { class: "card-title", text: "数据目录" }));
  const dataBody = h("div", { class: "mono", text: "读取中…" });
  const dataRow = h("div", { class: "row", style: "margin-top:10px" });
  const openDataBtn = h("button", { class: "btn", text: "打开数据目录" });
  openDataBtn.addEventListener("click", () => {
    api.dataDir().then((dir) => api.openFile(dir)).catch((e) => toast(errorMessage(e)));
  });
  dataRow.append(openDataBtn);
  dataCard.append(dataBody, dataRow);
  dataCard.append(
    h("p", {
      class: "list-meta",
      style: "margin-top:10px",
      text: "模板文件与数据库都保存在该目录下；应用升级、卸载重装都不会清空它。备份时直接复制整个目录即可。",
    })
  );
  page.append(dataCard);

  // ---------------- 隐私 ----------------
  const privacyCard = h("div", { class: "card" });
  privacyCard.append(h("h2", { class: "card-title", text: "数据处理原则" }));
  privacyCard.append(
    alertBox(
      "info",
      "合同内容、模板文件与填写的信息全部在本机处理，不会上传到任何服务器。\n" +
        "除「检查更新」与下载更新包外，应用不会发起网络请求；日志中不记录合同字段值与合同正文。"
    )
  );
  page.append(privacyCard);

  // ---------------- 生成记录 ----------------
  const historyCard = h("div", { class: "card" });
  historyCard.append(h("h2", { class: "card-title", text: "最近生成的合同" }));
  const historyArea = h("div", { class: "empty", text: "读取中…" });
  historyCard.append(historyArea);
  page.append(historyCard);

  root.append(page);

  // ---------------- 更新流程 ----------------
  let downloading = false;

  const offProgress = onUpdateProgress((p) => {
    if (p.phase === "download") {
      progressWrap.style.display = "";
      const pct = Math.max(0, Math.min(100, p.percent ?? 0));
      progressInner.style.width = `${pct.toFixed(1)}%`;
      const total = p.total && p.total > 0 ? ` / ${fmtSize(p.total)}` : "";
      progressText.textContent = `正在下载更新包… ${fmtSize(p.downloaded ?? 0)}${total}（${pct.toFixed(0)}%）`;
    } else if (p.phase === "verified") {
      progressInner.style.width = "100%";
      progressText.textContent = p.message ?? "下载完成，正在校验…";
    } else if (p.phase === "install") {
      progressText.textContent = p.message ?? "正在安装更新…";
    }
  });

  function setStatus(text: string): void {
    updateStatus.textContent = text;
  }

  function renderUpdate(info: UpdateInfo | null): void {
    updateArea.innerHTML = "";
    if (!info) return;
    if (info.upToDate) {
      updateArea.append(alertBox("success", `已是最新版本（${info.currentVersion}）。`));
      return;
    }
    if (!info.assetUrl) {
      // 有新版但当前平台没有安装包
      updateArea.append(alertBox("warn", info.message || "发现新版本，但没有适用于当前系统的安装包。"));
      if (info.releaseUrl) {
        const btn = h("button", { class: "btn", text: "前往发布页" });
        btn.addEventListener("click", () => {
          api.openReleasePage(info.releaseUrl).catch((e) => toast(errorMessage(e)));
        });
        updateArea.append(h("div", { class: "row", style: "margin-top:8px" }, [btn]));
      }
      return;
    }
    const box = h("div", { class: "alert info" });
    box.append(h("div", { text: `${info.message || "发现新版本"}：${info.currentVersion} → ${info.latestVersion}` }));
    if (info.publishedAt) {
      box.append(h("div", { class: "list-meta", text: `发布时间：${fmtTime(info.publishedAt)}` }));
    }
    if (info.notes) {
      const notes = h("pre", {
        style: "white-space:pre-wrap; margin:8px 0 0; font-size:12px; color:#374151; max-height:180px; overflow:auto",
        text: info.notes,
      });
      box.append(notes);
    }
    const actions = h("div", { class: "row", style: "margin-top:10px" });
    const sizeText = info.assetSize > 0 ? `（${fmtSize(info.assetSize)}）` : "";
    const installBtn = h("button", { class: "btn primary", text: `下载并更新${sizeText}` });
    installBtn.addEventListener("click", () => {
      void startUpdate(info, installBtn);
    });
    const manualBtn = h("button", { class: "btn", text: "前往发布页" });
    manualBtn.addEventListener("click", () => {
      api.openReleasePage(info.releaseUrl).catch((e) => toast(errorMessage(e)));
    });
    actions.append(installBtn, manualBtn);
    box.append(actions);
    updateArea.append(box);
  }

  async function startUpdate(info: UpdateInfo, btn: HTMLButtonElement): Promise<void> {
    if (downloading) return;
    downloading = true;
    btn.toggleAttribute("disabled", true);
    alertArea.innerHTML = "";
    progressWrap.style.display = "";
    progressInner.style.width = "0%";
    progressText.textContent = "正在下载更新包…";
    try {
      const path = await api.downloadUpdate(info.assetUrl, info.assetName, info.assetSize, info.sha256);
      progressText.textContent = "校验通过，正在安装…";
      const res = await api.applyUpdate(path);
      if (res.requiresManual) {
        alertArea.append(alertBox("warn", res.message));
        progressWrap.style.display = "none";
      } else {
        alertArea.append(alertBox("success", res.message));
        progressText.textContent = res.needsRestart ? "更新完成，应用即将重启…" : "更新完成，请手动重新打开应用。";
      }
    } catch (e) {
      alertArea.append(alertBox("error", `更新失败：${errorMessage(e)}（可以重试，当前版本不受影响）`));
      progressWrap.style.display = "none";
    } finally {
      downloading = false;
      btn.toggleAttribute("disabled", false);
    }
  }

  checkBtn.addEventListener("click", () => {
    void runCheck(true);
  });

  async function runCheck(userInitiated: boolean): Promise<void> {
    if (userInitiated) {
      setStatus("正在检查…");
      alertArea.innerHTML = "";
    }
    try {
      const info = await api.checkUpdate();
      setStatus(info ? `上次检查：${fmtTime(new Date().toISOString())}` : "");
      renderUpdate(info);
    } catch (e) {
      if (userInitiated) {
        alertArea.append(alertBox("error", `检查更新失败：${errorMessage(e)}`));
      }
      setStatus("");
    }
  }

  autoCheckBox.addEventListener("change", () => {
    api.setAutoCheckUpdate(autoCheckBox.checked).catch((e) => toast(errorMessage(e)));
  });

  // ---------------- 记录渲染 ----------------
  function renderHistory(items: ContractRecord[]): void {
    historyArea.className = "";
    historyArea.innerHTML = "";
    if (items.length === 0) {
      historyArea.className = "empty";
      historyArea.textContent = "还没有生成记录。";
      return;
    }
    const list = h("div", { class: "list" });
    for (const c of items) {
      const row = h("div", { class: "list-row" });
      const main = h("div", { class: "list-main" });
      main.append(h("div", { class: "list-name", text: c.fileName }));
      main.append(h("div", { class: "list-meta", text: `${c.templateName}｜${fmtTime(c.createdAt)}` }));
      main.append(h("div", { class: "mono", text: c.filePath }));
      row.append(main);
      const actions = h("div", { class: "row-actions" });
      const openBtn = h("button", { class: "btn link", text: "打开" });
      openBtn.addEventListener("click", () => {
        api.openFile(c.filePath).catch((e) => toast(errorMessage(e)));
      });
      const revealBtn = h("button", { class: "btn link", text: "定位" });
      revealBtn.addEventListener("click", () => {
        api.revealInFolder(c.filePath).catch((e) => toast(errorMessage(e)));
      });
      actions.append(openBtn, revealBtn);
      row.append(actions);
      list.append(row);
    }
    historyArea.append(list);
  }

  void (async () => {
    try {
      const info: VersionInfo = await api.versionInfo();
      versionBody.textContent = `版本：${info.version}\n提交：${info.commit}\n构建时间：${info.buildTime}`;
    } catch (e) {
      versionBody.textContent = errorMessage(e);
    }
    try {
      dataBody.textContent = await api.dataDir();
    } catch (e) {
      dataBody.textContent = errorMessage(e);
    }
    try {
      autoCheckBox.checked = await api.autoCheckUpdate();
      if (autoCheckBox.checked) {
        void runCheck(false);
      }
    } catch {
      autoCheckBox.checked = false;
    }
    try {
      renderHistory(await api.listContracts(20));
    } catch (e) {
      historyArea.textContent = `读取生成记录失败：${errorMessage(e)}`;
    }
  })();

  return () => {
    offProgress();
  };
}

/** 订阅更新进度事件；返回取消订阅函数。 */
function onUpdateProgress(cb: (p: UpdateProgress) => void): () => void {
  const w = window as unknown as {
    runtime?: { EventsOn: (name: string, fn: (payload: unknown) => void) => void; EventsOff?: (name: string) => void };
  };
  const rt = w.runtime;
  if (!rt) return () => {};
  rt.EventsOn("update:progress", (payload) => {
    if (payload && typeof payload === "object") {
      cb(payload as UpdateProgress);
    }
  });
  return () => {
    if (rt.EventsOff) rt.EventsOff("update:progress");
  };
}
