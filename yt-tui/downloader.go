package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type JobStatus string

const (
	StatusPending     JobStatus = "Ready"
	StatusQueued      JobStatus = "Queued"
	StatusFetching    JobStatus = "Fetching Info"
	StatusDownloading JobStatus = "Downloading"
	StatusMerging     JobStatus = "Processing"
	StatusCompleted   JobStatus = "Completed"
	StatusFailed      JobStatus = "Failed"
	StatusCancelled   JobStatus = "Cancelled"
)

type DownloadJob struct {
	ID         int
	URL        string
	Title      string
	Channel    string
	Duration   string
	Quality    QualityPreset
	SaveDir    string
	Status     JobStatus
	Percent    float64
	Speed      string
	ETA        string
	Size       string
	OutputFile string
	ErrorMsg   string
	CreatedAt  time.Time
	CancelFunc context.CancelFunc
}

type JobProgressMsg struct {
	JobID      int
	Title      string
	Status     JobStatus
	Percent    float64
	Speed      string
	ETA        string
	Size       string
	OutputFile string
	ErrorMsg   string
}

type VideoMetadataMsg struct {
	URL      string
	Title    string
	Duration string
	Channel  string
	Err      error
}

var (
	// Matches standard yt-dlp progress lines like:
	// [download]  45.2% of   52.30MiB at    4.12MiB/s ETA 00:07
	progressRegex = regexp.MustCompile(`\[download\]\s+(\d+(?:\.\d+)?)%\s+of\s+~?\s*([^\s]+)(?:\s+at\s+([^\s]+))?(?:\s+ETA\s+([^\s]+))?`)
	destRegex     = regexp.MustCompile(`\[(?:download|Merger|ExtractAudio)\]\s+(?:Destination:\s+|Merging formats into\s+"?)([^"\r\n]+)`)
)

func CheckYtDlpInstalled(binPath string) (string, bool) {
	if binPath == "" {
		binPath = "yt-dlp"
	}
	if binPath != "yt-dlp" {
		if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
			return binPath, true
		}
	}
	path, err := exec.LookPath(binPath)
	if err == nil {
		return path, true
	}
	// Check managed yt-tui bin path and common locations
	candidates := []string{
		managedYtDlpPath(),
		"yt-dlp.exe",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, "scoop", "shims", "yt-dlp.exe"),
			filepath.Join(home, "AppData", "Local", "Microsoft", "WinGet", "Links", "yt-dlp.exe"),
		)
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return binPath, false
}

func FetchMetadataCmd(binPath, url string) tea.Cmd {
	return func() tea.Msg {
		resolved, ok := CheckYtDlpInstalled(binPath)
		if !ok {
			return VideoMetadataMsg{URL: url, Err: fmt.Errorf("yt-dlp not found in PATH")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, resolved,
			"--no-playlist",
			"--no-warnings",
			"--print", "%(title)s\n%(duration_string)s\n%(uploader)s",
			url,
		)
		out, err := cmd.Output()
		if err != nil {
			return VideoMetadataMsg{URL: url, Err: err}
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		msg := VideoMetadataMsg{URL: url}
		if len(lines) > 0 {
			msg.Title = strings.TrimSpace(lines[0])
		}
		if len(lines) > 1 {
			msg.Duration = strings.TrimSpace(lines[1])
		}
		if len(lines) > 2 {
			msg.Channel = strings.TrimSpace(lines[2])
		}
		return msg
	}
}

type ProgressHub struct {
	mu sync.Mutex
	ch chan JobProgressMsg
}

func NewProgressHub() *ProgressHub {
	return &ProgressHub{
		ch: make(chan JobProgressMsg, 100),
	}
}

func (h *ProgressHub) ListenCmd() tea.Cmd {
	return func() tea.Msg {
		msg := <-h.ch
		return msg
	}
}

func (h *ProgressHub) Send(msg JobProgressMsg) {
	select {
	case h.ch <- msg:
	default:
	}
}

func StartDownloadWorker(ctx context.Context, hub *ProgressHub, binPath string, job *DownloadJob) {
	go func() {
		resolved, ok := CheckYtDlpInstalled(binPath)
		if !ok {
			hub.Send(JobProgressMsg{
				JobID:  job.ID,
				Status: StatusFetching,
				Title:  "Auto-downloading yt-dlp binary...",
			})
			target := managedYtDlpPath()
			if err := downloadYtDlpBinary(target); err != nil {
				hub.Send(JobProgressMsg{
					JobID:    job.ID,
					Status:   StatusFailed,
					ErrorMsg: "Failed to auto-download yt-dlp: " + err.Error(),
				})
				return
			}
			resolved = target
		}

		_ = os.MkdirAll(job.SaveDir, 0755)

		// First fetch title if unknown
		title := job.Title
		if title == "" || title == job.URL {
			hub.Send(JobProgressMsg{
				JobID:  job.ID,
				Status: StatusFetching,
			})
			titleCmd := exec.CommandContext(ctx, resolved, "--no-playlist", "--no-warnings", "--get-title", job.URL)
			if out, err := titleCmd.Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
					title = strings.TrimSpace(lines[0])
				}
			}
		}
		if title == "" {
			title = job.URL
		}

		hub.Send(JobProgressMsg{
			JobID:  job.ID,
			Title:  title,
			Status: StatusDownloading,
		})

		outputTemplate := filepath.Join(job.SaveDir, "%(title)s.%(ext)s")
		args := []string{
			"--newline",
			"--no-colors",
			"--no-playlist",
			"-o", outputTemplate,
		}
		args = append(args, job.Quality.FormatArgs...)
		args = append(args, job.URL)

		cmd := exec.CommandContext(ctx, resolved, args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			hub.Send(JobProgressMsg{
				JobID:    job.ID,
				Title:    title,
				Status:   StatusFailed,
				ErrorMsg: err.Error(),
			})
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			hub.Send(JobProgressMsg{
				JobID:    job.ID,
				Title:    title,
				Status:   StatusFailed,
				ErrorMsg: err.Error(),
			})
			return
		}

		if err := cmd.Start(); err != nil {
			hub.Send(JobProgressMsg{
				JobID:    job.ID,
				Title:    title,
				Status:   StatusFailed,
				ErrorMsg: err.Error(),
			})
			return
		}

		var lastErrLine string
		var outputFile string
		var lastPct float64

		combined := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(combined)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "ERROR:") {
				lastErrLine = strings.TrimPrefix(line, "ERROR:")
				lastErrLine = strings.TrimSpace(lastErrLine)
			}

			if m := destRegex.FindStringSubmatch(line); len(m) > 1 {
				outputFile = strings.Trim(m[1], `" `)
			}

			if strings.Contains(line, "[Merger]") || strings.Contains(line, "[ExtractAudio]") {
				hub.Send(JobProgressMsg{
					JobID:      job.ID,
					Title:      title,
					Status:     StatusMerging,
					Percent:    100,
					OutputFile: outputFile,
				})
				continue
			}

			if m := progressRegex.FindStringSubmatch(line); len(m) > 1 {
				pct, err := strconv.ParseFloat(m[1], 64)
				if err == nil {
					lastPct = pct
				}
				size := m[2]
				speed := ""
				eta := ""
				if len(m) > 3 {
					speed = m[3]
				}
				if len(m) > 4 {
					eta = m[4]
				}
				hub.Send(JobProgressMsg{
					JobID:      job.ID,
					Title:      title,
					Status:     StatusDownloading,
					Percent:    lastPct,
					Size:       size,
					Speed:      speed,
					ETA:        eta,
					OutputFile: outputFile,
				})
			}
		}

		err = cmd.Wait()
		if ctx.Err() == context.Canceled {
			hub.Send(JobProgressMsg{
				JobID:  job.ID,
				Title:  title,
				Status: StatusCancelled,
			})
			return
		}
		if err != nil {
			if lastErrLine == "" {
				lastErrLine = err.Error()
			}
			hub.Send(JobProgressMsg{
				JobID:    job.ID,
				Title:    title,
				Status:   StatusFailed,
				ErrorMsg: lastErrLine,
			})
			return
		}

		hub.Send(JobProgressMsg{
			JobID:      job.ID,
			Title:      title,
			Status:     StatusCompleted,
			Percent:    100,
			OutputFile: outputFile,
		})
	}()
}

