export namespace bridge {
	
	export class Item {
	    url: string;
	    fileName: string;
	    referer: string;
	    cookies: string;
	    userAgent: string;
	    headers: Record<string, string>;
	    kind: string;
	    size: number;
	    mime: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.referer = source["referer"];
	        this.cookies = source["cookies"];
	        this.userAgent = source["userAgent"];
	        this.headers = source["headers"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.mime = source["mime"];
	    }
	}
	export class AddRequest {
	    items: Item[];
	    batch: boolean;
	    silent: boolean;
	    page: string;
	
	    static createFrom(source: any = {}) {
	        return new AddRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], Item);
	        this.batch = source["batch"];
	        this.silent = source["silent"];
	        this.page = source["page"];
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
	export class Config {
	    enabled: boolean;
	    types: string[];
	    skip: string[];
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.types = source["types"];
	        this.skip = source["skip"];
	        this.version = source["version"];
	    }
	}

}

export namespace config {
	
	export class Category {
	    name: string;
	    extensions: string[];
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.extensions = source["extensions"];
	        this.dir = source["dir"];
	    }
	}
	export class Schedule {
	    enabled: boolean;
	    startTime: string;
	    stopTime: string;
	    days: number[];
	    onComplete: string;
	
	    static createFrom(source: any = {}) {
	        return new Schedule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.startTime = source["startTime"];
	        this.stopTime = source["stopTime"];
	        this.days = source["days"];
	        this.onComplete = source["onComplete"];
	    }
	}
	export class Queue {
	    id: string;
	    name: string;
	    maxConcurrent: number;
	    schedule: Schedule;
	
	    static createFrom(source: any = {}) {
	        return new Queue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.maxConcurrent = source["maxConcurrent"];
	        this.schedule = this.convertValues(source["schedule"], Schedule);
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
	export class SMBCredential {
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    domain: string;
	
	    static createFrom(source: any = {}) {
	        return new SMBCredential(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.domain = source["domain"];
	    }
	}
	
	export class Settings {
	    defaultDir: string;
	    maxConnections: number;
	    globalSpeedLimit: number;
	    maxRetries: number;
	    retryDelaySec: number;
	    timeoutSec: number;
	    proxy: string;
	    userAgent: string;
	    bridgePort: number;
	    bridgeToken: string;
	    showAddDialog: boolean;
	    showCompleteDialog: boolean;
	    clipboardMonitor: boolean;
	    browserIntercept: boolean;
	    interceptTypes: string[];
	    skipSites: string[];
	    ffmpegPath: string;
	    ytDlpPath: string;
	    antivirusCmd: string;
	    theme: string;
	    categories: Category[];
	    queues: Queue[];
	    smb: SMBCredential[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultDir = source["defaultDir"];
	        this.maxConnections = source["maxConnections"];
	        this.globalSpeedLimit = source["globalSpeedLimit"];
	        this.maxRetries = source["maxRetries"];
	        this.retryDelaySec = source["retryDelaySec"];
	        this.timeoutSec = source["timeoutSec"];
	        this.proxy = source["proxy"];
	        this.userAgent = source["userAgent"];
	        this.bridgePort = source["bridgePort"];
	        this.bridgeToken = source["bridgeToken"];
	        this.showAddDialog = source["showAddDialog"];
	        this.showCompleteDialog = source["showCompleteDialog"];
	        this.clipboardMonitor = source["clipboardMonitor"];
	        this.browserIntercept = source["browserIntercept"];
	        this.interceptTypes = source["interceptTypes"];
	        this.skipSites = source["skipSites"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ytDlpPath = source["ytDlpPath"];
	        this.antivirusCmd = source["antivirusCmd"];
	        this.theme = source["theme"];
	        this.categories = this.convertValues(source["categories"], Category);
	        this.queues = this.convertValues(source["queues"], Queue);
	        this.smb = this.convertValues(source["smb"], SMBCredential);
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

export namespace grabber {
	
	export class Item {
	    url: string;
	    saveAs: string;
	    referer: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.saveAs = source["saveAs"];
	        this.referer = source["referer"];
	        this.type = source["type"];
	    }
	}
	export class Options {
	    startUrl: string;
	    mode: string;
	    depth: number;
	    sameHost: boolean;
	    stayBelow: boolean;
	    include: string[];
	    exclude: string[];
	    maxPages: number;
	    concurrency: number;
	    cookies: string;
	    userAgent: string;
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startUrl = source["startUrl"];
	        this.mode = source["mode"];
	        this.depth = source["depth"];
	        this.sameHost = source["sameHost"];
	        this.stayBelow = source["stayBelow"];
	        this.include = source["include"];
	        this.exclude = source["exclude"];
	        this.maxPages = source["maxPages"];
	        this.concurrency = source["concurrency"];
	        this.cookies = source["cookies"];
	        this.userAgent = source["userAgent"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}

}

export namespace main {
	
	export class GrabberAddRequest {
	    items: grabber.Item[];
	    dir: string;
	    queueId: string;
	    startNow: boolean;
	    cookies: string;
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new GrabberAddRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], grabber.Item);
	        this.dir = source["dir"];
	        this.queueId = source["queueId"];
	        this.startNow = source["startNow"];
	        this.cookies = source["cookies"];
	        this.username = source["username"];
	        this.password = source["password"];
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
	export class Info {
	    version: string;
	    initError: string;
	    bridgeError: string;
	    ffmpeg: string;
	    ytdlp: string;
	    configDir: string;
	    os: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.initError = source["initError"];
	        this.bridgeError = source["bridgeError"];
	        this.ffmpeg = source["ffmpeg"];
	        this.ytdlp = source["ytdlp"];
	        this.configDir = source["configDir"];
	        this.os = source["os"];
	    }
	}

}

export namespace model {
	
	export class AddRequest {
	    url: string;
	    fileName: string;
	    dir: string;
	    category: string;
	    queueId: string;
	    kind: string;
	    referer: string;
	    userAgent: string;
	    cookies: string;
	    headers: Record<string, string>;
	    username: string;
	    password: string;
	    description: string;
	    connections: number;
	    speedLimit: number;
	    postProcess: string;
	    startNow: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new AddRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.dir = source["dir"];
	        this.category = source["category"];
	        this.queueId = source["queueId"];
	        this.kind = source["kind"];
	        this.referer = source["referer"];
	        this.userAgent = source["userAgent"];
	        this.cookies = source["cookies"];
	        this.headers = source["headers"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.description = source["description"];
	        this.connections = source["connections"];
	        this.speedLimit = source["speedLimit"];
	        this.postProcess = source["postProcess"];
	        this.startNow = source["startNow"];
	        this.size = source["size"];
	    }
	}
	export class Segment {
	    start: number;
	    end: number;
	    pos: number;
	    active?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Segment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.pos = source["pos"];
	        this.active = source["active"];
	    }
	}
	export class Download {
	    id: string;
	    url: string;
	    fileName: string;
	    dir: string;
	    category: string;
	    queueId: string;
	    kind: string;
	    status: string;
	    error?: string;
	    size: number;
	    downloaded: number;
	    resumable: boolean;
	    connections: number;
	    segments?: Segment[];
	    referer?: string;
	    userAgent?: string;
	    cookies?: string;
	    headers?: Record<string, string>;
	    username?: string;
	    password?: string;
	    description?: string;
	    speedLimit: number;
	    postProcess?: string;
	    // Go type: time
	    added: any;
	    // Go type: time
	    lastTry: any;
	    // Go type: time
	    finished: any;
	    speed: number;
	    eta: number;
	    activeConns: number;
	
	    static createFrom(source: any = {}) {
	        return new Download(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.dir = source["dir"];
	        this.category = source["category"];
	        this.queueId = source["queueId"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.size = source["size"];
	        this.downloaded = source["downloaded"];
	        this.resumable = source["resumable"];
	        this.connections = source["connections"];
	        this.segments = this.convertValues(source["segments"], Segment);
	        this.referer = source["referer"];
	        this.userAgent = source["userAgent"];
	        this.cookies = source["cookies"];
	        this.headers = source["headers"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.description = source["description"];
	        this.speedLimit = source["speedLimit"];
	        this.postProcess = source["postProcess"];
	        this.added = this.convertValues(source["added"], null);
	        this.lastTry = this.convertValues(source["lastTry"], null);
	        this.finished = this.convertValues(source["finished"], null);
	        this.speed = source["speed"];
	        this.eta = source["eta"];
	        this.activeConns = source["activeConns"];
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
	export class ProbeResult {
	    url: string;
	    finalUrl: string;
	    fileName: string;
	    size: number;
	    resumable: boolean;
	    contentType: string;
	    kind: string;
	    category: string;
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new ProbeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.finalUrl = source["finalUrl"];
	        this.fileName = source["fileName"];
	        this.size = source["size"];
	        this.resumable = source["resumable"];
	        this.contentType = source["contentType"];
	        this.kind = source["kind"];
	        this.category = source["category"];
	        this.dir = source["dir"];
	    }
	}

}

