package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"todo/internal/scanner"
	"todo/internal/store"
)

func TestStoreAndScannerEndToEnd(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// 1. Create a manual task first -> TASK (001)
	manual, err := st.CreateTask(ctx, store.CreateTaskInput{
		Title:    "Write release notes",
		Priority: "high",
		Category: "Docs",
	})
	if err != nil {
		t.Fatalf("create manual task: %v", err)
	}
	if manual.TaskCode != "TASK (001)" {
		t.Fatalf("expected TASK (001), got %s", manual.TaskCode)
	}

	// Subtasks CRUD
	sub, err := st.CreateSubtask(ctx, manual.ID, "Draft changelog")
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	sub, err = st.ToggleSubtask(ctx, sub.ID)
	if err != nil || !sub.Completed {
		t.Fatalf("expected subtask completed, got %+v, err=%v", sub, err)
	}

	// 2. Prepare temp codebase with TODOs
	tmpDir := t.TempDir()

	goFile := filepath.Join(tmpDir, "main.go")
	goContent := "package main\n\n// TODO: implement auth middleware\nfunc main() {\n\t// Normal comment with todoList variable\n\t// TODO - add graceful shutdown\n}\n"
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	pyFile := filepath.Join(tmpDir, "app.py")
	pyContent := "# TODO: parse CLI arguments\r\nprint('hello')\r\n"
	if err := os.WriteFile(pyFile, []byte(pyContent), 0o644); err != nil {
		t.Fatalf("write py file: %v", err)
	}

	// Ignored directory should be skipped
	ignoredDir := filepath.Join(tmpDir, "node_modules")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, "lib.js"), []byte("// TODO: should be ignored\n"), 0o644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	// Binary file should be skipped
	if err := os.WriteFile(filepath.Join(tmpDir, "bin.go"), []byte{0x00, 0x01, 'T', 'O', 'D', 'O'}, 0o644); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	// 3. Preview (Dry Run) should find 3 TODOs without modifying files or DB
	preview, err := scanner.Preview(ctx, st, scanner.ScanOptions{RootDir: tmpDir})
	if err != nil {
		t.Fatalf("preview scan: %v", err)
	}
	if !preview.DryRun {
		t.Fatalf("expected DryRun=true")
	}
	if preview.TodosFound != 3 {
		t.Fatalf("expected 3 TODOs in preview, got %d", preview.TodosFound)
	}
	if preview.TodosReplaced != 0 || preview.FilesModified != 0 {
		t.Fatalf("expected 0 replacements in preview, got replaced=%d modified=%d", preview.TodosReplaced, preview.FilesModified)
	}
	if preview.Matches[0].TaskCode != "TASK (002)" {
		t.Fatalf("expected first preview code TASK (002), got %s", preview.Matches[0].TaskCode)
	}

	// Verify DB still has only 1 task
	tasksBefore, err := st.ListTasks(ctx)
	if err != nil || len(tasksBefore) != 1 {
		t.Fatalf("expected 1 task before apply, got %d (err=%v)", len(tasksBefore), err)
	}

	// 4. Apply ScanAndReplace
	applied, err := scanner.ScanAndReplace(ctx, st, scanner.ScanOptions{
		RootDir:  tmpDir,
		Category: "Backend",
		Priority: "high",
	})
	if err != nil {
		t.Fatalf("scan and replace: %v", err)
	}
	if applied.TodosFound != 3 || applied.TodosReplaced != 3 || applied.FilesModified != 2 {
		t.Fatalf("unexpected apply counts: %+v", applied)
	}

	// Verify files were updated properly
	updatedGo, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatalf("read updated go file: %v", err)
	}
	if strings.Contains(string(updatedGo), "TODO") {
		t.Fatalf("expected no TODO remaining in main.go, got:\n%s", string(updatedGo))
	}
	if !strings.Contains(string(updatedGo), "TASK (") {
		t.Fatalf("expected TASK (XXX) in main.go, got:\n%s", string(updatedGo))
	}
	if !strings.Contains(string(updatedGo), "todoList") {
		t.Fatalf("expected todoList identifier to remain untouched")
	}

	updatedPy, err := os.ReadFile(pyFile)
	if err != nil {
		t.Fatalf("read updated py file: %v", err)
	}
	if !strings.Contains(string(updatedPy), "\r\n") {
		t.Fatalf("expected CRLF line endings to be preserved in app.py")
	}
	if strings.Contains(string(updatedPy), "TODO") {
		t.Fatalf("expected no TODO remaining in app.py, got:\n%s", string(updatedPy))
	}

	// 5. Re-running scan should find 0 TODOs
	second, err := scanner.ScanAndReplace(ctx, st, scanner.ScanOptions{RootDir: tmpDir})
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if second.TodosFound != 0 || second.TodosReplaced != 0 {
		t.Fatalf("expected 0 TODOs on second scan, got %+v", second)
	}

	// 6. Verify DB has 4 tasks total (1 manual + 3 scanned) and scan history has 2 records
	allTasks, err := st.ListTasks(ctx)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(allTasks) != 4 {
		t.Fatalf("expected 4 tasks in db, got %d", len(allTasks))
	}

	stats, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.Total != 4 || stats.ScannedCount != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	history, err := st.ListScanHistory(ctx)
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 scan history entries, got %d (err=%v)", len(history), err)
	}
}
