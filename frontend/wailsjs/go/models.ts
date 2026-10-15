export namespace contract {
	
	export class FieldDef {
	    key: string;
	    label: string;
	    hint: string;
	    maxRunes: number;
	
	    static createFrom(source: any = {}) {
	        return new FieldDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.hint = source["hint"];
	        this.maxRunes = source["maxRunes"];
	    }
	}
	export class FieldError {
	    key: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new FieldError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.message = source["message"];
	    }
	}
	export class GenerateRequest {
	    templateId: string;
	    values: Record<string, string>;
	    outputDir: string;
	    fileName: string;
	    outputFormat: string;
	    keepDocxCopy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GenerateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.templateId = source["templateId"];
	        this.values = source["values"];
	        this.outputDir = source["outputDir"];
	        this.fileName = source["fileName"];
	        this.outputFormat = source["outputFormat"];
	        this.keepDocxCopy = source["keepDocxCopy"];
	    }
	}
	export class GenerateResult {
	    filePath: string;
	    fileName: string;
	    dir: string;
	    sha256: string;
	    templateName: string;
	    // Go type: time
	    createdAt: any;
	    format: string;
	    warning: string;
	    backupPath: string;
	    converterName: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filePath = source["filePath"];
	        this.fileName = source["fileName"];
	        this.dir = source["dir"];
	        this.sha256 = source["sha256"];
	        this.templateName = source["templateName"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.format = source["format"];
	        this.warning = source["warning"];
	        this.backupPath = source["backupPath"];
	        this.converterName = source["converterName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace converter {
	
	export class ChannelStatus {
	    kind: string;
	    name: string;
	    available: boolean;
	    path: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new ChannelStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.available = source["available"];
	        this.path = source["path"];
	        this.detail = source["detail"];
	    }
	}
	export class Status {
	    available: boolean;
	    kind: string;
	    name: string;
	    path: string;
	    detail: string;
	    channels: ChannelStatus[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.detail = source["detail"];
	        this.channels = this.convertValues(source["channels"], ChannelStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace store {
	
	export class Contract {
	    id: string;
	    templateId: string;
	    templateName: string;
	    fileName: string;
	    filePath: string;
	    sha256: string;
	    fieldNames: string[];
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Contract(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.templateId = source["templateId"];
	        this.templateName = source["templateName"];
	        this.fileName = source["fileName"];
	        this.filePath = source["filePath"];
	        this.sha256 = source["sha256"];
	        this.fieldNames = source["fieldNames"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PlaceholderInfo {
	    name: string;
	    count: number;
	    parts: string[];
	
	    static createFrom(source: any = {}) {
	        return new PlaceholderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	        this.parts = source["parts"];
	    }
	}
	export class Template {
	    id: string;
	    name: string;
	    sizeBytes: number;
	    sha256: string;
	    placeholders: PlaceholderInfo[];
	    fieldUsage: Record<string, number>;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    fileExists: boolean;
	    sourceFormat: string;
	    originalFile: string;
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sha256 = source["sha256"];
	        this.placeholders = this.convertValues(source["placeholders"], PlaceholderInfo);
	        this.fieldUsage = source["fieldUsage"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.fileExists = source["fileExists"];
	        this.sourceFormat = source["sourceFormat"];
	        this.originalFile = source["originalFile"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace template {
	
	export class PlaceholderView {
	    name: string;
	    count: number;
	    parts: string[];
	    unknown: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PlaceholderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	        this.parts = source["parts"];
	        this.unknown = source["unknown"];
	    }
	}
	export class Detail {
	    template: store.Template;
	    placeholders: PlaceholderView[];
	    problems: string[];
	    missing: string[];
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Detail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.template = this.convertValues(source["template"], store.Template);
	        this.placeholders = this.convertValues(source["placeholders"], PlaceholderView);
	        this.problems = source["problems"];
	        this.missing = source["missing"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace version {
	
	export class Info {
	    version: string;
	    commit: string;
	    buildTime: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.buildTime = source["buildTime"];
	    }
	}

}

