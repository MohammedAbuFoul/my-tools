package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type QualityPreset struct {
	ID          string
	Name        string
	Description string
	FormatArgs  []string
}

var QualityPresets = []QualityPreset{
	{
		ID:          "best",
		Name:        "Best Available (Video + Audio)",
		Description: "Highest resolution MP4/MKV available",
		FormatArgs:  []string{"-f", "bv*+ba/b", "--merge-output-format", "mp4"},
	},
	{
		ID:          "1080p",
		Name:        "1080p Full HD (MP4)",
		Description: "Up to 1080p video with best audio",
		FormatArgs:  []string{"-f", "bv*[height<=1080]+ba/b[height<=1080]/b", "--merge-output-format", "mp4"},
	},
	{
		ID:          "720p",
		Name:        "720p HD (MP4)",
		Description: "Up to 720p video with best audio",
		FormatArgs:  []string{"-f", "bv*[height<=720]+ba/b[height<=720]/b", "--merge-output-format", "mp4"},
	},
	{
		ID:          "480p",
		Name:        "480p SD (MP4)",
		Description: "Up to 480p video with best audio",
		FormatArgs:  []string{"-f", "bv*[height<=480]+ba/b[height<=480]/b", "--merge-output-format", "mp4"},
	},
	{
		ID:          "mp3",
		Name:        "Audio Only (MP3)",
		Description: "Extract audio and convert to MP3",
		FormatArgs:  []string{"-x", "--audio-format", "mp3", "--audio-quality", "0"},
	},
	{
		ID:          "m4a",
		Name:        "Audio Only (M4A)",
		Description: "Extract best M4A audio stream",
		FormatArgs:  []string{"-f", "ba[ext=m4a]/ba", "-x", "--audio-format", "m4a"},
	},
}

type Config struct {
	DefaultQuality string `json:"default_quality"`
	DownloadDir    string `json:"download_dir"`
	AutoClipboard  bool   `json:"auto_clipboard"`
	YtDlpPath      string `json:"yt_dlp_path"`
}

func configFilePath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "yt-tui-config.json"
	}
	appDir := filepath.Join(cfgDir, "yt-tui")
	_ = os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "config.json")
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	dl := filepath.Join(home, "Downloads/video")
	if info, err := os.Stat(dl); err == nil && info.IsDir() {
		return dl
	}
	return home
}

func LoadConfig() Config {
	cfg := Config{
		DefaultQuality: "480p",
		DownloadDir:    defaultDownloadDir(),
		AutoClipboard:  true,
		YtDlpPath:      "yt-dlp",
	}

	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.DefaultQuality == "" {
		cfg.DefaultQuality = "480p"
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = defaultDownloadDir()
	}
	if cfg.YtDlpPath == "" {
		cfg.YtDlpPath = "yt-dlp"
	}
	return cfg
}

func SaveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath(), data, 0644)
}

func GetPresetByID(id string) QualityPreset {
	for _, p := range QualityPresets {
		if p.ID == id {
			return p
		}
	}
	return QualityPresets[1] // fallback 1080p
}

