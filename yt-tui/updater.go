package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type YtDlpStatusMsg struct {
	Installing bool
	Updating   bool
	Ready      bool
	Version    string
	BinPath    string
	Message    string
	Err        error
}

func managedYtDlpPath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		if runtime.GOOS == "windows" {
			return "yt-dlp.exe"
		}
		return "yt-dlp"
	}
	binDir := filepath.Join(cfgDir, "yt-tui", "bin")
	_ = os.MkdirAll(binDir, 0755)
	if runtime.GOOS == "windows" {
		return filepath.Join(binDir, "yt-dlp.exe")
	}
	return filepath.Join(binDir, "yt-dlp")
}

func ytDlpDownloadURL() string {
	switch runtime.GOOS {
	case "windows":
		return "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
	case "darwin":
		return "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
	default:
		return "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
	}
}

func getYtDlpVersion(binPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binPath, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func downloadYtDlpBinary(destPath string) error {
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)
	tmpPath := destPath + ".tmp"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ytDlpDownloadURL(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "yt-tui-downloader/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading yt-dlp", resp.StatusCode)
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpPath, 0755)
	}

	_ = os.Remove(destPath)
	return os.Rename(tmpPath, destPath)
}

// EnsureAndUpdateYtDlpCmd checks if yt-dlp exists:
// - If missing, downloads the latest official release binary automatically.
// - If present, runs `yt-dlp -U` to self-update to the latest release.
func EnsureAndUpdateYtDlpCmd(configuredPath string, forceUpdate bool) tea.Cmd {
	return func() tea.Msg {
		resolved, ok := CheckYtDlpInstalled(configuredPath)
		if !ok {
			target := managedYtDlpPath()
			if err := downloadYtDlpBinary(target); err != nil {
				return YtDlpStatusMsg{
					Ready:   false,
					Message: "Failed to auto-download yt-dlp",
					Err:     err,
				}
			}
			ver := getYtDlpVersion(target)
			return YtDlpStatusMsg{
				Ready:   true,
				Version: ver,
				BinPath: target,
				Message: fmt.Sprintf("Downloaded yt-dlp (%s) automatically!", ver),
			}
		}

		// Already installed -> run self-update (`yt-dlp -U`)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, resolved, "-U").CombinedOutput()
		outputStr := strings.TrimSpace(string(out))

		// If self-update fails because of permissions or package manager lock and forceUpdate is requested,
		// fallback to downloading into our managed app directory if using default path.
		if err != nil && forceUpdate && (configuredPath == "" || configuredPath == "yt-dlp") {
			target := managedYtDlpPath()
			if dlErr := downloadYtDlpBinary(target); dlErr == nil {
				ver := getYtDlpVersion(target)
				return YtDlpStatusMsg{
					Ready:   true,
					Version: ver,
					BinPath: target,
					Message: fmt.Sprintf("Updated yt-dlp to %s!", ver),
				}
			}
		}

		ver := getYtDlpVersion(resolved)
		msg := fmt.Sprintf("yt-dlp %s is up to date", ver)
		if strings.Contains(outputStr, "Updated yt-dlp to") {
			msg = fmt.Sprintf("Updated yt-dlp to %s!", ver)
		}
		return YtDlpStatusMsg{
			Ready:   true,
			Version: ver,
			BinPath: resolved,
			Message: msg,
		}
	}
}

