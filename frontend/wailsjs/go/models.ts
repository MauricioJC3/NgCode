export namespace main {
	
	export class DirEntry {
	    Name: string;
	    Path: string;
	    IsDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DirEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Path = source["Path"];
	        this.IsDir = source["IsDir"];
	    }
	}
	export class FileData {
	    Path: string;
	    Content: string;
	
	    static createFrom(source: any = {}) {
	        return new FileData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Content = source["Content"];
	    }
	}
	export class GitDiffResult {
	    path: string;
	    added: number[];
	    modified: number[];
	    removed: number[];
	
	    static createFrom(source: any = {}) {
	        return new GitDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.added = source["added"];
	        this.modified = source["modified"];
	        this.removed = source["removed"];
	    }
	}

}

