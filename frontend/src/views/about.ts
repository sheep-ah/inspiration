import { api, errorMessage, type ContractRecord, type VersionInfo } from "../api";
import { alertBox, fmtTime, h, toast } from "../ui";

/** 关于页：版本信息、数据目录、隐私说明与最近的生成记录。 */
export function renderAbout(root: HTMLElement): () => void {
  const page = h("div");
  page.append(
    h("h1", { class: "page-title", text: "关于" }),
    h("p", { class: "page-desc", text: "版本信息、数据存放位置与本地处理说明。" })
  );

  const alertArea = h("div");
  page.append(alertArea);

  // 版本
  const versionCard = h("div", { class: "card" });
  versionCard.append(h("h2", { class: "card-title", text: "版本信息" }));
  const versionBody = h("div", { class: "mono", text: "读取中…" });
  versionCard.append(versionBody);
  page.append(versionCard);

  // 数据目录
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

  // 隐私
  const privacyCard = h("div", { class: "card" });
  privacyCard.append(h("h2", { class: "card-title", text: "数据处理原则" }));
  privacyCard.append(
    alertBox(
      "info",
      "合同内容、模板文件与填写的信息全部在本机处理，不会上传到任何服务器。\n除「检查更新」外，应用不会发起网络请求；日志中不记录合同字段值与合同正文。"
    )
  );
  page.append(privacyCard);

  // 生成记录
  const historyCard = h("div", { class: "card" });
  historyCard.append(h("h2", { class: "card-title", text: "最近生成的合同" }));
  const historyArea = h("div", { class: "empty", text: "读取中…" });
  historyCard.append(historyArea);
  page.append(historyCard);

  root.append(page);

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
      versionBody.textContent =
        `版本：${info.version}\n` + `提交：${info.commit}\n` + `构建时间：${info.buildTime}`;
      versionBody.style.whiteSpace = "pre-wrap";
    } catch (e) {
      versionBody.textContent = errorMessage(e);
    }
    try {
      dataBody.textContent = await api.dataDir();
    } catch (e) {
      dataBody.textContent = errorMessage(e);
    }
    try {
      renderHistory(await api.listContracts(20));
    } catch (e) {
      historyArea.textContent = `读取生成记录失败：${errorMessage(e)}`;
    }
  })();

  return () => {
    /* 本页没有需要解绑的全局监听 */
  };
}
