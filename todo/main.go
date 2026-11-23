package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"

	"todo/internal/scanner"
	"todo/internal/store"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		os.Exit(runCLIScan(os.Args[2:]))
	}

	st, err := store.Open("")
	if err != nil {
		log.Fatalf("failed to open sqlite database: %v", err)
	}

	launchGUI(st, nil)
}

func launchGUI(st *store.Store, initialScan *scanner.ScanResult) {
	app := NewApp(st, initialScan)

	err := wails.Run(&options.App{
		Title:     "TaskFlow — Code TODO Scanner & To-Do App",
		Width:     1280,
		Height:    840,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatalf("wails error: %v", err)
	}
}

func runCLIScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "Preview matches without modifying files or database (does not launch GUI)")
	noGUI := fs.Bool("no-gui", false, "Scan and exit without launching the desktop app")
	extFlag := fs.String("ext", "", "Comma-separated file extensions (e.g. .go,.py,.ts or * for all text files)")
	ignoreFlag := fs.String("ignore", "", "Additional comma-separated directory names to ignore")
	categoryFlag := fs.String("category", "", "Category to assign to created tasks")
	priorityFlag := fs.String("priority", "medium", "Priority for created tasks (low, medium, high, urgent)")
	dbFlag := fs.String("db", "", "Path to SQLite database file (defaults to app database)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	targetDir := "."
	if fs.NArg() >= 1 {
		targetDir = fs.Arg(0)
	}

	st, err := store.Open(*dbFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		return 1
	}

	opts := scanner.ScanOptions{
		RootDir:    targetDir,
		Extensions: parseCSVList(*extFlag),
		IgnoreDirs: parseCSVList(*ignoreFlag),
		Category:   *categoryFlag,
		Priority:   *priorityFlag,
	}

	ctx := context.Background()
	var res scanner.ScanResult
	if *dryRun {
		res, err = scanner.Preview(ctx, st, opts)
	} else {
		res, err = scanner.ScanAndReplace(ctx, st, opts)
	}
	if err != nil {
		_ = st.Close()
		fmt.Fprintln(os.Stderr, formatScanCLIError(err))
		return 1
	}

	mode := "APPLIED"
	if res.DryRun {
		mode = "DRY-RUN"
	}
	fmt.Printf("[%s] Scanned %d files in %s — found %d TODO(s), replaced %d across %d file(s)\n",
		mode, res.FilesScanned, res.RootDir, res.TodosFound, res.TodosReplaced, res.FilesModified)
	for _, m := range res.Matches {
		fmt.Printf("  %s | %s:%d | %s\n", m.TaskCode, m.RelativePath, m.LineNumber, m.Title)
	}
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "  warning: %s\n", e)
	}

	if *dryRun || *noGUI {
		_ = st.Close()
		return 0
	}

	launchGUI(st, &res)
	return 0
}
