export namespace scanner {
	
	export class MatchItem {
	    file_path: string;
	    relative_path: string;
	    line_number: number;
	    original_line: string;
	    replaced_line: string;
	    task_code: string;
	    title: string;
	    task_id: number;
	
	    static createFrom(source: any = {}) {
	        return new MatchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_path = source["file_path"];
	        this.relative_path = source["relative_path"];
	        this.line_number = source["line_number"];
	        this.original_line = source["original_line"];
	        this.replaced_line = source["replaced_line"];
	        this.task_code = source["task_code"];
	        this.title = source["title"];
	        this.task_id = source["task_id"];
	    }
	}
	export class ScanOptions {
	    root_dir: string;
	    extensions: string[];
	    ignore_dirs: string[];
	    category: string;
	    priority: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root_dir = source["root_dir"];
	        this.extensions = source["extensions"];
	        this.ignore_dirs = source["ignore_dirs"];
	        this.category = source["category"];
	        this.priority = source["priority"];
	    }
	}
	export class ScanResult {
	    root_dir: string;
	    dry_run: boolean;
	    files_scanned: number;
	    files_modified: number;
	    todos_found: number;
	    todos_replaced: number;
	    matches: MatchItem[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root_dir = source["root_dir"];
	        this.dry_run = source["dry_run"];
	        this.files_scanned = source["files_scanned"];
	        this.files_modified = source["files_modified"];
	        this.todos_found = source["todos_found"];
	        this.todos_replaced = source["todos_replaced"];
	        this.matches = this.convertValues(source["matches"], MatchItem);
	        this.errors = source["errors"];
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
	
	export class CreateTaskInput {
	    title: string;
	    description: string;
	    status: string;
	    priority: string;
	    category: string;
	    due_date: string;
	    source_file: string;
	    source_line: number;
	    source_snippet: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateTaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.category = source["category"];
	        this.due_date = source["due_date"];
	        this.source_file = source["source_file"];
	        this.source_line = source["source_line"];
	        this.source_snippet = source["source_snippet"];
	    }
	}
	export class ScanHistoryDTO {
	    id: number;
	    root_dir: string;
	    files_scanned: number;
	    todos_found: number;
	    todos_replaced: number;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanHistoryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.root_dir = source["root_dir"];
	        this.files_scanned = source["files_scanned"];
	        this.todos_found = source["todos_found"];
	        this.todos_replaced = source["todos_replaced"];
	        this.created_at = source["created_at"];
	    }
	}
	export class StatsDTO {
	    total: number;
	    todo_count: number;
	    in_progress_count: number;
	    done_count: number;
	    scanned_count: number;
	
	    static createFrom(source: any = {}) {
	        return new StatsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.todo_count = source["todo_count"];
	        this.in_progress_count = source["in_progress_count"];
	        this.done_count = source["done_count"];
	        this.scanned_count = source["scanned_count"];
	    }
	}
	export class SubtaskDTO {
	    id: number;
	    task_id: number;
	    title: string;
	    completed: boolean;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SubtaskDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.title = source["title"];
	        this.completed = source["completed"];
	        this.created_at = source["created_at"];
	    }
	}
	export class TaskDTO {
	    id: number;
	    task_number: number;
	    task_code: string;
	    title: string;
	    description: string;
	    status: string;
	    priority: string;
	    category: string;
	    due_date: string;
	    source_file: string;
	    source_line: number;
	    source_snippet: string;
	    created_at: string;
	    updated_at: string;
	    completed_at: string;
	    subtasks: SubtaskDTO[];
	
	    static createFrom(source: any = {}) {
	        return new TaskDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_number = source["task_number"];
	        this.task_code = source["task_code"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.category = source["category"];
	        this.due_date = source["due_date"];
	        this.source_file = source["source_file"];
	        this.source_line = source["source_line"];
	        this.source_snippet = source["source_snippet"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.completed_at = source["completed_at"];
	        this.subtasks = this.convertValues(source["subtasks"], SubtaskDTO);
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
	export class UpdateTaskInput {
	    id: number;
	    title: string;
	    description: string;
	    status: string;
	    priority: string;
	    category: string;
	    due_date: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateTaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.category = source["category"];
	        this.due_date = source["due_date"];
	    }
	}

}

