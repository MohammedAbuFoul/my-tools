package scanner

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"todo/internal/store"
)

const maxFileSizeBytes = 5 * 1024 * 1024 // 5MB

var todoRegex = regexp.MustCompile(`\bTODO\b`)

var DefaultIgnoreDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"bin":          true,
	"obj":          true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".idea":        true,
	".vscode":      true,
	".next":        true,
	".nuxt":        true,
	"coverage":     true,
}

var DefaultCodeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".java": true, ".kt": true, ".kts": true, ".c": true, ".h": true, ".cpp": true,
	".hpp": true, ".cc": true, ".cs": true, ".rs": true, ".rb": true, ".php": true,
	".swift": true, ".scala": true, ".sh": true, ".bash": true, ".zsh": true,
	".ps1": true, ".sql": true, ".html": true, ".htm": true, ".css": true,
	".scss": true, ".less": true, ".vue": true, ".svelte": true, ".yaml": true,
	".yml": true, ".toml": true, ".lua": true, ".r": true, ".dart": true,
	".ex": true, ".exs": true, ".zig": true, ".nim": true, ".tf": true,
	".hcl": true, ".proto": true, ".graphql": true, ".md": true,
}

type ScanOptions struct {
	RootDir    string   `json:"root_dir"`
	Extensions []string `json:"extensions"`
	IgnoreDirs []string `json:"ignore_dirs"`
	Category   string   `json:"category"`
	Priority   string   `json:"priority"`
}

type MatchItem struct {
	FilePath     string `json:"file_path"`
	RelativePath string `json:"relative_path"`
	LineNumber   int64  `json:"line_number"`
	OriginalLine string `json:"original_line"`
	ReplacedLine string `json:"replaced_line"`
	TaskCode     string `json:"task_code"`
	Title        string `json:"title"`
	TaskID       int64  `json:"task_id"`
}

type ScanResult struct {
	RootDir       string      `json:"root_dir"`
	DryRun        bool        `json:"dry_run"`
	FilesScanned  int64       `json:"files_scanned"`
	FilesModified int64       `json:"files_modified"`
	TodosFound    int64       `json:"todos_found"`
	TodosReplaced int64       `json:"todos_replaced"`
	Matches       []MatchItem `json:"matches"`
	Errors        []string    `json:"errors"`
}

func Preview(ctx context.Context, st *store.Store, opts ScanOptions) (ScanResult, error) {
	return runScan(ctx, st, opts, true)
}

func ScanAndReplace(ctx context.Context, st *store.Store, opts ScanOptions) (ScanResult, error) {
	return runScan(ctx, st, opts, false)
}

func runScan(ctx context.Context, st *store.Store, opts ScanOptions, dryRun bool) (ScanResult, error) {
	rootDir := strings.TrimSpace(opts.RootDir)
	if rootDir == "" {
		return ScanResult{}, fmt.Errorf("root directory is required")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return ScanResult{}, fmt.Errorf("resolve root dir: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return ScanResult{}, fmt.Errorf("stat root dir: %w", err)
	}
	if !info.IsDir() {
		return ScanResult{}, fmt.Errorf("path is not a directory: %s", absRoot)
	}

	ignoreSet := buildIgnoreSet(opts.IgnoreDirs)
	extSet, scanAllText := buildExtSet(opts.Extensions)

	category := strings.TrimSpace(opts.Category)
	if category == "" {
		category = filepath.Base(absRoot)
		if category == "" || category == "." || category == string(filepath.Separator) {
			category = "Code"
		}
	}
	priority := strings.TrimSpace(opts.Priority)
	if priority == "" {
		priority = "medium"
	}

	var previewCounter int64
	if dryRun {
		cur, err := st.CurrentTaskNumber(ctx)
		if err != nil {
			return ScanResult{}, fmt.Errorf("get task counter: %w", err)
		}
		previewCounter = cur
	}

	result := ScanResult{
		RootDir: absRoot,
		DryRun:  dryRun,
		Matches: []MatchItem{},
		Errors:  []string{},
	}

	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if path != absRoot && ignoreSet[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			return nil
		}

		if !scanAllText {
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if !extSet[ext] {
				return nil
			}
		}

		fileInfo, err := d.Info()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if fileInfo.Size() == 0 || fileInfo.Size() > maxFileSizeBytes {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if isBinary(raw) {
			return nil
		}

		result.FilesScanned++

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		lineEnding := "\n"
		if bytes.Contains(raw, []byte("\r\n")) {
			lineEnding = "\r\n"
		}
		content := strings.ReplaceAll(string(raw), "\r\n", "\n")
		lines := strings.Split(content, "\n")

		fileModified := false
		for i, line := range lines {
			loc := todoRegex.FindStringIndex(line)
			if loc == nil {
				continue
			}

			result.TodosFound++
			lineNum := int64(i + 1)
			title := extractTitle(line, loc[1], relPath, lineNum)

			var taskCode string
			var taskID int64
			var replacedLine string

			if dryRun {
				previewCounter++
				taskCode = store.FormatTaskCode(previewCounter)
				replacedLine = line[:loc[0]] + taskCode + line[loc[1]:]
			} else {
				created, err := st.CreateTask(ctx, store.CreateTaskInput{
					Title:         title,
					Description:   fmt.Sprintf("Extracted from %s:%d", relPath, lineNum),
					Status:        "todo",
					Priority:      priority,
					Category:      category,
					SourceFile:    path,
					SourceLine:    lineNum,
					SourceSnippet: strings.TrimSpace(line),
				})
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s:%d: %v", relPath, lineNum, err))
					continue
				}
				taskCode = created.TaskCode
				taskID = created.ID
				replacedLine = line[:loc[0]] + taskCode + line[loc[1]:]
				lines[i] = replacedLine
				fileModified = true
				result.TodosReplaced++
			}

			result.Matches = append(result.Matches, MatchItem{
				FilePath:     path,
				RelativePath: relPath,
				LineNumber:   lineNum,
				OriginalLine: strings.TrimSpace(line),
				ReplacedLine: strings.TrimSpace(replacedLine),
				TaskCode:     taskCode,
				Title:        title,
				TaskID:       taskID,
			})
		}

		if !dryRun && fileModified {
			updated := strings.Join(lines, lineEnding)
			if err := os.WriteFile(path, []byte(updated), fileInfo.Mode().Perm()); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("write %s: %v", relPath, err))
				return nil
			}
			result.FilesModified++
		}

		return nil
	})
	if walkErr != nil {
		return result, walkErr
	}

	if !dryRun {
		_, _ = st.RecordScanHistory(ctx, absRoot, result.FilesScanned, result.TodosFound, result.TodosReplaced)
	}

	return result, nil
}

func extractTitle(line string, afterTodoIdx int, relPath string, lineNum int64) string {
	rest := strings.TrimSpace(line[afterTodoIdx:])
	// Strip common trailing block comment markers
	for _, suffix := range []string{"*/", "-->", "#>", "-}}"} {
		rest = strings.TrimSuffix(rest, suffix)
	}
	rest = strings.TrimSpace(rest)
	// Strip leading separator punctuation like :, -, —
	rest = strings.TrimLeft(rest, ":-–— \t")
	rest = strings.TrimSpace(rest)

	if rest == "" {
		return fmt.Sprintf("TODO at %s:%d", relPath, lineNum)
	}
	return rest
}

func isBinary(data []byte) bool {
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	return bytes.IndexByte(data[:checkLen], 0) != -1
}

func buildIgnoreSet(extra []string) map[string]bool {
	set := make(map[string]bool, len(DefaultIgnoreDirs)+len(extra))
	for k, v := range DefaultIgnoreDirs {
		set[strings.ToLower(k)] = v
	}
	for _, d := range extra {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			set[d] = true
		}
	}
	return set
}

func buildExtSet(exts []string) (map[string]bool, bool) {
	if len(exts) == 0 {
		return DefaultCodeExtensions, false
	}
	set := make(map[string]bool, len(exts))
	for _, ext := range exts {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if ext == "*" || ext == ".*" {
			return nil, true
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		set[ext] = true
	}
	if len(set) == 0 {
		return DefaultCodeExtensions, false
	}
	return set, false
}
