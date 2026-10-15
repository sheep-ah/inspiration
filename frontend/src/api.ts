/**
 * 与 Go 绑定层的类型化封装。
 *
 * Wails 会在运行时注入 window.go.<package>.<Struct>.<Method>；
 * 这里显式声明类型，前端因此不依赖 wailsjs 生成的中间文件。
 */

export interface FieldDef {
  key: string;
  label: string;
  hint: string;
  maxRunes: number;
}

export interface FieldError {
  key: string;
  message: string;
}

export interface Template {
  id: string;
  name: string;
  sizeBytes: number;
  sha256: string;
  placeholders: PlaceholderInfo[];
  fieldUsage: Record<string, number>;
  createdAt: string;
  updatedAt: string;
  fileExists: boolean;
  /** docx=直接上传；doc=由 .doc 自动转换而来 */
  sourceFormat: "docx" | "doc";
  /** 由 .doc 转换时保留的原始文件名（空表示无） */
  originalFile: string;
}

export interface PlaceholderInfo {
  name: string;
  count: number;
  parts: string[];
}

export interface PlaceholderView extends PlaceholderInfo {
  unknown: boolean;
}

export interface TemplateDetail {
  template: Template;
  placeholders: PlaceholderView[];
  problems: string[] | null;
  missing: string[] | null;
  /** 导入成功但需要注意的提示（例如由 .doc 转换而来、未识别到占位符） */
  warning: string;
}

export interface GenerateRequest {
  templateId: string;
  values: Record<string, string>;
  outputDir: string;
  fileName: string;
  /** docx（默认）或 doc（需要本机 Word/WPS/LibreOffice） */
  outputFormat: "docx" | "doc";
  /** 输出 .doc 时是否同时保留一份 .docx 备份（推荐） */
  keepDocxCopy: boolean;
}

export interface GenerateResult {
  filePath: string;
  fileName: string;
  dir: string;
  sha256: string;
  templateName: string;
  createdAt: string;
  /** 实际产出的格式：docx / doc */
  format: "docx" | "doc";
  /** 同时保留的 .docx 备份路径（未开启或未生成时为空） */
  backupPath: string;
  /** 实际使用的转换通道名称（Microsoft Word / WPS Office / LibreOffice） */
  converterName: string;
  /** 非空表示"文件已生成但有需要注意的地方"（如 .doc 转换失败已降级为 .docx） */
  warning: string;
}

export interface ConverterChannel {
  kind: "word" | "wps" | "libreoffice";
  name: string;
  available: boolean;
  path: string;
  detail: string;
}

export interface ConverterStatus {
  available: boolean;
  /** 将会使用的通道 */
  kind: string;
  name: string;
  path: string;
  detail: string;
  /** 全部通道及可用性 */
  channels: ConverterChannel[];
}

export interface ContractRecord {
  id: string;
  templateId: string;
  templateName: string;
  fileName: string;
  filePath: string;
  sha256: string;
  fieldNames: string[] | null;
  createdAt: string;
}

export interface VersionInfo {
  version: string;
  commit: string;
  buildTime: string;
}

interface AppBridge {
  VersionInfo(): Promise<VersionInfo>;
  DataDir(): Promise<string>;
  FieldDefs(): Promise<FieldDef[]>;
  ValidateForm(values: Record<string, string>): Promise<FieldError[]>;
  ListTemplates(query: string): Promise<Template[]>;
  ImportTemplate(): Promise<TemplateDetail | null>;
  TemplateDetail(id: string): Promise<TemplateDetail>;
  RenameTemplate(id: string, name: string): Promise<void>;
  DeleteTemplate(id: string): Promise<void>;
  DefaultOutputDir(): Promise<string>;
  ConverterStatus(): Promise<ConverterStatus>;
  PickOutputDir(current: string): Promise<string>;
  GenerateContract(req: GenerateRequest): Promise<GenerateResult>;
  ListContracts(limit: number): Promise<ContractRecord[]>;
  OpenFile(path: string): Promise<void>;
  RevealInFolder(path: string): Promise<void>;
  SupportedFields(): Promise<string[]>;
}

interface RuntimeBridge {
  EventsOn(event: string, cb: (payload: unknown) => void): void;
}

function bridge(): AppBridge {
  const w = window as unknown as { go?: { app?: { App?: AppBridge } } };
  const app = w.go?.app?.App;
  if (!app) {
    throw new Error("应用绑定未就绪，请通过 Wails 启动应用");
  }
  return app;
}

function runtime(): RuntimeBridge | undefined {
  return (window as unknown as { runtime?: RuntimeBridge }).runtime;
}

/** 统一把 Go 侧错误转换为可展示的中文消息。 */
export function errorMessage(err: unknown): string {
  if (typeof err === "string") return err;
  if (err && typeof err === "object" && "message" in err) {
    return String((err as { message: unknown }).message);
  }
  return "操作失败，请重试";
}

export const api = {
  versionInfo: () => bridge().VersionInfo(),
  dataDir: () => bridge().DataDir(),
  fieldDefs: () => bridge().FieldDefs(),
  validateForm: (values: Record<string, string>) => bridge().ValidateForm(values),
  listTemplates: (query: string) => bridge().ListTemplates(query),
  importTemplate: () => bridge().ImportTemplate(),
  templateDetail: (id: string) => bridge().TemplateDetail(id),
  renameTemplate: (id: string, name: string) => bridge().RenameTemplate(id, name),
  deleteTemplate: (id: string) => bridge().DeleteTemplate(id),
  defaultOutputDir: () => bridge().DefaultOutputDir(),
  converterStatus: () => bridge().ConverterStatus(),
  supportedFields: () => bridge().SupportedFields(),
  pickOutputDir: (current: string) => bridge().PickOutputDir(current),
  generateContract: (req: GenerateRequest) => bridge().GenerateContract(req),
  listContracts: (limit: number) => bridge().ListContracts(limit),
  openFile: (path: string) => bridge().OpenFile(path),
  revealInFolder: (path: string) => bridge().RevealInFolder(path),
};

export const events = {
  on: (name: string, cb: (payload: unknown) => void) => runtime()?.EventsOn(name, cb),
};
