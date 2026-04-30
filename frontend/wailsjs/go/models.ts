export namespace main {
	
	export class AppFeatures {
	    baiduUpscale: boolean;
	    upscaleHint?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppFeatures(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baiduUpscale = source["baiduUpscale"];
	        this.upscaleHint = source["upscaleHint"];
	    }
	}
	export class CropRect {
	    x0: number;
	    y0: number;
	    x1: number;
	    y1: number;
	
	    static createFrom(source: any = {}) {
	        return new CropRect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x0 = source["x0"];
	        this.y0 = source["y0"];
	        this.x1 = source["x1"];
	        this.y1 = source["y1"];
	    }
	}
	export class TileCoord {
	    col: number;
	    row: number;
	
	    static createFrom(source: any = {}) {
	        return new TileCoord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.col = source["col"];
	        this.row = source["row"];
	    }
	}
	export class ExportRequest {
	    crop?: CropRect;
	    targetWidthCm: number;
	    targetHeightCm: number;
	    paperWidthMm: number;
	    paperHeightMm: number;
	    landscape?: boolean;
	    marginMm: number;
	    overlapMm: number;
	    mode?: string;
	    allowRotate?: boolean;
	    skippedTiles?: TileCoord[];
	    upscale: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.crop = this.convertValues(source["crop"], CropRect);
	        this.targetWidthCm = source["targetWidthCm"];
	        this.targetHeightCm = source["targetHeightCm"];
	        this.paperWidthMm = source["paperWidthMm"];
	        this.paperHeightMm = source["paperHeightMm"];
	        this.landscape = source["landscape"];
	        this.marginMm = source["marginMm"];
	        this.overlapMm = source["overlapMm"];
	        this.mode = source["mode"];
	        this.allowRotate = source["allowRotate"];
	        this.skippedTiles = this.convertValues(source["skippedTiles"], TileCoord);
	        this.upscale = source["upscale"];
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
	export class ExportResponse {
	    outputPath: string;
	    pages: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputPath = source["outputPath"];
	        this.pages = source["pages"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class ImageInfo {
	    path: string;
	    format: string;
	    width: number;
	    height: number;
	    dpiX: number;
	    dpiY: number;
	    rawDpiX: number;
	    rawDpiY: number;
	    previewWidth: number;
	    previewHeight: number;
	    previewDataUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.format = source["format"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.dpiX = source["dpiX"];
	        this.dpiY = source["dpiY"];
	        this.rawDpiX = source["rawDpiX"];
	        this.rawDpiY = source["rawDpiY"];
	        this.previewWidth = source["previewWidth"];
	        this.previewHeight = source["previewHeight"];
	        this.previewDataUrl = source["previewDataUrl"];
	    }
	}
	export class PackedTileView {
	    col: number;
	    row: number;
	    x0: number;
	    y0: number;
	    x1: number;
	    y1: number;
	    xMm: number;
	    yMm: number;
	    placeWMm: number;
	    placeHMm: number;
	    rotated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackedTileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.col = source["col"];
	        this.row = source["row"];
	        this.x0 = source["x0"];
	        this.y0 = source["y0"];
	        this.x1 = source["x1"];
	        this.y1 = source["y1"];
	        this.xMm = source["xMm"];
	        this.yMm = source["yMm"];
	        this.placeWMm = source["placeWMm"];
	        this.placeHMm = source["placeHMm"];
	        this.rotated = source["rotated"];
	    }
	}
	export class PackedPageView {
	    index: number;
	    landscape: boolean;
	    usableWMm: number;
	    usableHMm: number;
	    tiles: PackedTileView[];
	
	    static createFrom(source: any = {}) {
	        return new PackedPageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.landscape = source["landscape"];
	        this.usableWMm = source["usableWMm"];
	        this.usableHMm = source["usableHMm"];
	        this.tiles = this.convertValues(source["tiles"], PackedTileView);
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
	
	export class PlanRequest {
	    crop?: CropRect;
	    targetWidthCm?: number;
	    targetHeightCm?: number;
	    paperWidthMm: number;
	    paperHeightMm: number;
	    marginMm: number;
	    overlapMm: number;
	    mode?: string;
	    allowRotate?: boolean;
	    skippedTiles?: TileCoord[];
	
	    static createFrom(source: any = {}) {
	        return new PlanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.crop = this.convertValues(source["crop"], CropRect);
	        this.targetWidthCm = source["targetWidthCm"];
	        this.targetHeightCm = source["targetHeightCm"];
	        this.paperWidthMm = source["paperWidthMm"];
	        this.paperHeightMm = source["paperHeightMm"];
	        this.marginMm = source["marginMm"];
	        this.overlapMm = source["overlapMm"];
	        this.mode = source["mode"];
	        this.allowRotate = source["allowRotate"];
	        this.skippedTiles = this.convertValues(source["skippedTiles"], TileCoord);
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
	export class PlanTileView {
	    col: number;
	    row: number;
	    x0: number;
	    y0: number;
	    x1: number;
	    y1: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanTileView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.col = source["col"];
	        this.row = source["row"];
	        this.x0 = source["x0"];
	        this.y0 = source["y0"];
	        this.x1 = source["x1"];
	        this.y1 = source["y1"];
	    }
	}
	export class PlanResponse {
	    cols: number;
	    rows: number;
	    sourceW: number;
	    sourceH: number;
	    tilePxW: number;
	    tilePxH: number;
	    stepPxX: number;
	    stepPxY: number;
	    overlapPxX: number;
	    overlapPxY: number;
	    tiles: PlanTileView[];
	    packedPages?: PackedPageView[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new PlanResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	        this.sourceW = source["sourceW"];
	        this.sourceH = source["sourceH"];
	        this.tilePxW = source["tilePxW"];
	        this.tilePxH = source["tilePxH"];
	        this.stepPxX = source["stepPxX"];
	        this.stepPxY = source["stepPxY"];
	        this.overlapPxX = source["overlapPxX"];
	        this.overlapPxY = source["overlapPxY"];
	        this.tiles = this.convertValues(source["tiles"], PlanTileView);
	        this.packedPages = this.convertValues(source["packedPages"], PackedPageView);
	        this.warnings = source["warnings"];
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

