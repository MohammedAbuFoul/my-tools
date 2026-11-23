package main

import (
	"context"
	"fmt"
	"strings"

	"todo/internal/scanner"
	"todo/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	store       *store.Store
	initialScan *scanner.ScanResult
}

func NewApp(st *store.Store, initialScan *scanner.ScanResult) *App {
	return &App{
		store:       st,
		initialScan: initialScan,
	}
}

func (a *App) GetInitialScan() *scanner.ScanResult {
	return a.initialScan
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) GetDBPath() string {
	return a.store.DBPath()
}

func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Codebase Folder to Scan for TODOs",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

func (a *App) ListTasks() ([]store.TaskDTO, error) {
	return a.store.ListTasks(a.ctx)
}

func (a *App) GetTask(id int64) (store.TaskDTO, error) {
	return a.store.GetTask(a.ctx, id)
}

func (a *App) CreateTask(in store.CreateTaskInput) (store.TaskDTO, error) {
	return a.store.CreateTask(a.ctx, in)
}

func (a *App) UpdateTask(in store.UpdateTaskInput) (store.TaskDTO, error) {
	return a.store.UpdateTask(a.ctx, in)
}

func (a *App) UpdateTaskStatus(id int64, status string) (store.TaskDTO, error) {
	return a.store.UpdateTaskStatus(a.ctx, id, status)
}

func (a *App) DeleteTask(id int64) error {
	return a.store.DeleteTask(a.ctx, id)
}

func (a *App) DeleteCompletedTasks() error {
	return a.store.DeleteCompletedTasks(a.ctx)
}

func (a *App) GetStats() (store.StatsDTO, error) {
	return a.store.GetStats(a.ctx)
}

func (a *App) ListCategories() ([]string, error) {
	return a.store.ListCategories(a.ctx)
}

func (a *App) CreateSubtask(taskID int64, title string) (store.SubtaskDTO, error) {
	return a.store.CreateSubtask(a.ctx, taskID, title)
}

func (a *App) ToggleSubtask(id int64) (store.SubtaskDTO, error) {
	return a.store.ToggleSubtask(a.ctx, id)
}

func (a *App) DeleteSubtask(id int64) error {
	return a.store.DeleteSubtask(a.ctx, id)
}

func (a *App) PreviewScan(opts scanner.ScanOptions) (scanner.ScanResult, error) {
	return scanner.Preview(a.ctx, a.store, opts)
}

func (a *App) ScanAndReplace(opts scanner.ScanOptions) (scanner.ScanResult, error) {
	return scanner.ScanAndReplace(a.ctx, a.store, opts)
}

func (a *App) ListScanHistory() ([]store.ScanHistoryDTO, error) {
	return a.store.ListScanHistory(a.ctx)
}

func parseCSVList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func formatScanCLIError(err error) string {
	return fmt.Sprintf("scan failed: %v", err)
}
