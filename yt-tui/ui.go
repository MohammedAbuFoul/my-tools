package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type activeTab int

const (
	tabDownload activeTab = iota
	tabSettings
)

type downloadFocus int

const (
	focusURL downloadFocus = iota
	focusQuality
	focusQueue
)

type settingsFocus int

const (
	settingsFocusQuality settingsFocus = iota
	settingsFocusSaveDir
	settingsFocusClipboard
	settingsFocusYtDlpPath
)

type clipboardTickMsg time.Time

func clipboardPollCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		return clipboardTickMsg(t)
	})
}

type model struct {
	cfg           Config
	tab           activeTab
	dlFocus       downloadFocus
	setFocus      settingsFocus
	urlInput      textinput.Model
	dirInput      textinput.Model
	ytdlpInput    textinput.Model
	qualityCursor int
	setQualCursor int
	queueCursor   int
	jobs          []*DownloadJob
	nextJobID     int
	hub           *ProgressHub
	progBar       progress.Model

	lastClipURL   string
	clipBannerMsg string
	statusToast   string

	metaLoading bool
	metaURL     string
	metaTitle   string
	metaChannel string
	metaDur     string
	metaErr     string

	ytdlpChecking bool
	ytdlpReady    bool
	ytdlpVersion  string
	ytdlpErr      string

	width  int
	height int
}

func initialModel() model {
	cfg := LoadConfig()

	urlIn := textinput.New()
	urlIn.Placeholder = "https://www.youtube.com/watch?v=... (or copy a YouTube link)"
	urlIn.Focus()
	urlIn.CharLimit = 512
	urlIn.Width = 68

	dirIn := textinput.New()
	dirIn.Placeholder = "C:\\Users\\...\\Downloads"
	dirIn.SetValue(cfg.DownloadDir)
	dirIn.CharLimit = 512
	dirIn.Width = 60

	ytdlpIn := textinput.New()
	ytdlpIn.Placeholder = "yt-dlp (or full path to yt-dlp.exe)"
	ytdlpIn.SetValue(cfg.YtDlpPath)
	ytdlpIn.CharLimit = 512
	ytdlpIn.Width = 60

	qIdx := 1
	for i, p := range QualityPresets {
		if p.ID == cfg.DefaultQuality {
			qIdx = i
			break
		}
	}

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 40

	_, hasLocal := CheckYtDlpInstalled(cfg.YtDlpPath)

	m := model{
		cfg:           cfg,
		tab:           tabDownload,
		dlFocus:       focusURL,
		urlInput:      urlIn,
		dirInput:      dirIn,
		ytdlpInput:    ytdlpIn,
		qualityCursor: qIdx,
		setQualCursor: qIdx,
		nextJobID:     1,
		hub:           NewProgressHub(),
		progBar:       prog,
		ytdlpChecking: true,
		ytdlpReady:    hasLocal,
		width:         96,
		height:        30,
	}

	// Check clipboard on startup
	if clipURL, ok := ReadClipboardYouTubeURL(); ok {
		m.lastClipURL = clipURL
		m.urlInput.SetValue(clipURL)
		m.clipBannerMsg = "Auto-added YouTube URL from clipboard to queue!"
		m.metaLoading = true
		m.metaURL = clipURL
		m.addPendingJob(clipURL)
	}

	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textinput.Blink,
		m.hub.ListenCmd(),
		clipboardPollCmd(),
		EnsureAndUpdateYtDlpCmd(m.cfg.YtDlpPath, false),
	}
	if m.metaLoading && m.metaURL != "" && m.ytdlpReady {
		cmds = append(cmds, FetchMetadataCmd(m.cfg.YtDlpPath, m.metaURL))
	}
	return tea.Batch(cmds...)
}

func (m *model) addPendingJob(url string) *DownloadJob {
	url = strings.TrimSpace(url)
	if url == "" || !IsYouTubeURL(url) {
		return nil
	}
	// Avoid adding duplicate pending/active URL
	for _, j := range m.jobs {
		if j.URL == url && (j.Status == StatusPending || j.Status == StatusQueued || j.Status == StatusDownloading || j.Status == StatusFetching) {
			return j
		}
	}

	preset := QualityPresets[m.qualityCursor]
	title := url
	if m.metaURL == url && m.metaTitle != "" {
		title = m.metaTitle
	}

	job := &DownloadJob{
		ID:        m.nextJobID,
		URL:       url,
		Title:     title,
		Quality:   preset,
		SaveDir:   m.cfg.DownloadDir,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	m.nextJobID++
	m.jobs = append([]*DownloadJob{job}, m.jobs...)
	m.queueCursor = 0
	return job
}

func (m *model) startJob(job *DownloadJob) {
	if job == nil {
		return
	}
	if job.Status == StatusDownloading || job.Status == StatusFetching || job.Status == StatusMerging {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job.CancelFunc = cancel
	job.Status = StatusQueued
	job.Percent = 0
	job.ErrorMsg = ""
	job.SaveDir = m.cfg.DownloadDir
	StartDownloadWorker(ctx, m.hub, m.cfg.YtDlpPath, job)
}

func (m *model) startAllPendingJobs() int {
	count := 0
	for _, j := range m.jobs {
		if j.Status == StatusPending || j.Status == StatusFailed || j.Status == StatusCancelled {
			if j.Status == StatusPending {
				m.startJob(j)
				count++
			}
		}
	}
	return count
}

func (m *model) enqueueDownload(url string, startNow bool) tea.Cmd {
	url = strings.TrimSpace(url)
	if url == "" {
		m.statusToast = "Please enter or copy a YouTube URL first."
		return nil
	}
	if !IsYouTubeURL(url) {
		m.statusToast = "Invalid YouTube URL format."
		return nil
	}

	// Check if there is already a pending job for this URL
	for i, j := range m.jobs {
		if j.URL == url && j.Status == StatusPending {
			j.Quality = QualityPresets[m.qualityCursor]
			j.SaveDir = m.cfg.DownloadDir
			m.queueCursor = i
			if startNow {
				m.startJob(j)
				m.statusToast = fmt.Sprintf("Started download #%d (%s)", j.ID, j.Quality.Name)
			} else {
				m.statusToast = fmt.Sprintf("Updated pending item #%d (%s)", j.ID, j.Quality.Name)
			}
			return nil
		}
	}

	job := m.addPendingJob(url)
	if job == nil {
		return nil
	}
	if startNow {
		m.startJob(job)
		m.statusToast = fmt.Sprintf("Started download #%d (%s)", job.ID, job.Quality.Name)
	} else {
		m.statusToast = fmt.Sprintf("Added #%d to queue (Ready to start)", job.ID)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputW := msg.Width - 22
		if inputW < 30 {
			inputW = 30
		}
		if inputW > 85 {
			inputW = 85
		}
		m.urlInput.Width = inputW
		m.dirInput.Width = inputW - 6
		m.ytdlpInput.Width = inputW - 6
		return m, nil

	case clipboardTickMsg:
		if m.cfg.AutoClipboard {
			if clipURL, ok := ReadClipboardYouTubeURL(); ok && clipURL != m.lastClipURL {
				m.lastClipURL = clipURL
				m.urlInput.SetValue(clipURL)
				job := m.addPendingJob(clipURL)
				if job != nil {
					m.clipBannerMsg = fmt.Sprintf("Auto-added #%d from clipboard (Ready to start)", job.ID)
					m.statusToast = fmt.Sprintf("Added clipboard link to queue as #%d — press Enter or 'a' to start!", job.ID)
				} else {
					m.clipBannerMsg = "Detected YouTube link in clipboard!"
				}
				m.metaLoading = true
				m.metaURL = clipURL
				m.metaTitle = ""
				m.metaChannel = ""
				m.metaDur = ""
				m.metaErr = ""
				cmds = append(cmds, FetchMetadataCmd(m.cfg.YtDlpPath, clipURL))
			}
		}
		cmds = append(cmds, clipboardPollCmd())
		return m, tea.Batch(cmds...)

	case YtDlpStatusMsg:
		m.ytdlpChecking = false
		if msg.Err != nil {
			m.ytdlpReady = false
			m.ytdlpErr = msg.Err.Error()
			m.statusToast = "yt-dlp auto-download failed: " + msg.Err.Error()
			return m, nil
		}
		m.ytdlpReady = msg.Ready
		m.ytdlpVersion = msg.Version
		m.ytdlpErr = ""
		if msg.BinPath != "" && (m.cfg.YtDlpPath == "" || m.cfg.YtDlpPath == "yt-dlp") {
			m.cfg.YtDlpPath = msg.BinPath
			m.ytdlpInput.SetValue(msg.BinPath)
			_ = SaveConfig(m.cfg)
		}
		if msg.Message != "" {
			m.statusToast = msg.Message
		}
		// Fetch metadata for any pending jobs that don't have a title yet
		var fetchCmds []tea.Cmd
		if m.metaURL != "" && m.metaTitle == "" {
			m.metaLoading = true
			m.metaErr = ""
			fetchCmds = append(fetchCmds, FetchMetadataCmd(m.cfg.YtDlpPath, m.metaURL))
		}
		for _, j := range m.jobs {
			if j.Title == j.URL && j.URL != m.metaURL {
				fetchCmds = append(fetchCmds, FetchMetadataCmd(m.cfg.YtDlpPath, j.URL))
			}
		}
		return m, tea.Batch(fetchCmds...)

	case VideoMetadataMsg:
		if msg.Err == nil && msg.Title != "" {
			for _, j := range m.jobs {
				if j.URL == msg.URL {
					j.Title = msg.Title
					j.Duration = msg.Duration
					j.Channel = msg.Channel
				}
			}
		}
		if msg.URL == m.metaURL {
			m.metaLoading = false
			if msg.Err != nil {
				m.metaErr = msg.Err.Error()
			} else {
				m.metaTitle = msg.Title
				m.metaDur = msg.Duration
				m.metaChannel = msg.Channel
			}
		}
		return m, nil

	case JobProgressMsg:
		for _, j := range m.jobs {
			if j.ID == msg.JobID {
				if msg.Title != "" {
					j.Title = msg.Title
				}
				if msg.Status != "" {
					j.Status = msg.Status
				}
				if msg.Percent > 0 {
					j.Percent = msg.Percent
				}
				if msg.Speed != "" {
					j.Speed = msg.Speed
				}
				if msg.ETA != "" {
					j.ETA = msg.ETA
				}
				if msg.Size != "" {
					j.Size = msg.Size
				}
				if msg.OutputFile != "" {
					j.OutputFile = msg.OutputFile
				}
				if msg.ErrorMsg != "" {
					j.ErrorMsg = msg.ErrorMsg
				}
				break
			}
		}
		cmds = append(cmds, m.hub.ListenCmd())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			for _, j := range m.jobs {
				if j.CancelFunc != nil {
					j.CancelFunc()
				}
			}
			return m, tea.Quit

		case "q":
			// Only quit with 'q' if not typing inside a focused text input
			if !m.isTyping() {
				for _, j := range m.jobs {
					if j.CancelFunc != nil {
						j.CancelFunc()
					}
				}
				return m, tea.Quit
			}

		case "f1":
			m.switchTab(tabDownload)
			return m, nil

		case "f2", "ctrl+s":
			if m.tab == tabDownload {
				m.switchTab(tabSettings)
			} else {
				m.saveSettings()
				m.switchTab(tabDownload)
			}
			return m, nil

		case "ctrl+v", "alt+p":
			raw := ReadClipboardRaw()
			if raw != "" {
				if m.tab == tabDownload {
					m.urlInput.SetValue(raw)
					m.clipBannerMsg = "Pasted from clipboard"
					if IsYouTubeURL(raw) {
						m.lastClipURL = raw
						m.addPendingJob(raw)
						m.metaLoading = true
						m.metaURL = raw
						m.metaTitle = ""
						m.metaErr = ""
						cmds = append(cmds, FetchMetadataCmd(m.cfg.YtDlpPath, raw))
					}
				} else if m.tab == tabSettings && m.setFocus == settingsFocusSaveDir {
					m.dirInput.SetValue(raw)
				} else if m.tab == tabSettings && m.setFocus == settingsFocusYtDlpPath {
					m.ytdlpInput.SetValue(raw)
				}
			}
			return m, tea.Batch(cmds...)

		case "ctrl+u":
			if !m.ytdlpChecking {
				m.ytdlpChecking = true
				m.statusToast = "Checking for yt-dlp updates..."
				return m, EnsureAndUpdateYtDlpCmd(m.cfg.YtDlpPath, true)
			}
			return m, nil

		case "ctrl+f":
			if m.tab == tabDownload {
				u := strings.TrimSpace(m.urlInput.Value())
				if IsYouTubeURL(u) {
					m.metaLoading = true
					m.metaURL = u
					m.metaTitle = ""
					m.metaErr = ""
					m.statusToast = "Fetching video metadata..."
					return m, FetchMetadataCmd(m.cfg.YtDlpPath, u)
				}
				m.statusToast = "Enter a valid YouTube URL to fetch info."
				return m, nil
			}
		}

		if m.tab == tabDownload {
			return m.updateDownloadTab(msg)
		}
		return m.updateSettingsTab(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m model) isTyping() bool {
	if m.tab == tabDownload && m.dlFocus == focusURL {
		return true
	}
	if m.tab == tabSettings && (m.setFocus == settingsFocusSaveDir || m.setFocus == settingsFocusYtDlpPath) {
		return true
	}
	return false
}

func (m *model) switchTab(t activeTab) {
	m.tab = t
	m.urlInput.Blur()
	m.dirInput.Blur()
	m.ytdlpInput.Blur()
	if t == tabDownload {
		if m.dlFocus == focusURL {
			m.urlInput.Focus()
		}
	} else {
		if m.setFocus == settingsFocusSaveDir {
			m.dirInput.Focus()
		} else if m.setFocus == settingsFocusYtDlpPath {
			m.ytdlpInput.Focus()
		}
	}
}

func (m *model) saveSettings() {
	m.cfg.DefaultQuality = QualityPresets[m.setQualCursor].ID
	dir := strings.TrimSpace(m.dirInput.Value())
	if dir != "" {
		m.cfg.DownloadDir = dir
	}
	ytPath := strings.TrimSpace(m.ytdlpInput.Value())
	if ytPath != "" {
		m.cfg.YtDlpPath = ytPath
	}
	_ = SaveConfig(m.cfg)
	m.qualityCursor = m.setQualCursor
	m.statusToast = "Settings saved as default!"
}

func (m model) updateDownloadTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		if msg.String() == "tab" {
			m.dlFocus = (m.dlFocus + 1) % 3
		} else {
			m.dlFocus = (m.dlFocus + 2) % 3
		}
		if m.dlFocus == focusURL {
			m.urlInput.Focus()
		} else {
			m.urlInput.Blur()
		}
		return m, nil

	case "enter", "s":
		if msg.String() == "s" && m.dlFocus == focusURL {
			break // let 's' be typed into URL input
		}
		if m.dlFocus == focusQueue {
			if len(m.jobs) > 0 && m.queueCursor < len(m.jobs) {
				job := m.jobs[m.queueCursor]
				if job.Status == StatusPending || job.Status == StatusFailed || job.Status == StatusCancelled {
					m.startJob(job)
					m.statusToast = fmt.Sprintf("Started download #%d (%s)", job.ID, job.Quality.Name)
				} else {
					m.statusToast = fmt.Sprintf("Job #%d is already %s", job.ID, job.Status)
				}
			}
			return m, nil
		}
		if msg.String() == "enter" && (m.dlFocus == focusURL || m.dlFocus == focusQuality) {
			cmd := m.enqueueDownload(m.urlInput.Value(), true)
			return m, cmd
		}
		return m, nil

	case "a", "ctrl+a":
		if msg.String() == "a" && m.dlFocus == focusURL {
			break // let 'a' be typed into URL input
		}
		started := m.startAllPendingJobs()
		if started > 0 {
			m.statusToast = fmt.Sprintf("Started %d pending download(s)!", started)
		} else {
			m.statusToast = "No pending downloads waiting in queue."
		}
		return m, nil

	case "up", "k":
		if m.dlFocus == focusQuality {
			if m.qualityCursor > 0 {
				m.qualityCursor--
			}
			return m, nil
		} else if m.dlFocus == focusQueue {
			if m.queueCursor > 0 {
				m.queueCursor--
			}
			return m, nil
		}

	case "down", "j":
		if m.dlFocus == focusQuality {
			if m.qualityCursor < len(QualityPresets)-1 {
				m.qualityCursor++
			}
			return m, nil
		} else if m.dlFocus == focusQueue {
			if m.queueCursor < len(m.jobs)-1 {
				m.queueCursor++
			}
			return m, nil
		}

	case "left", "h", "right", "l":
		if m.dlFocus == focusQueue && len(m.jobs) > 0 && m.queueCursor < len(m.jobs) {
			job := m.jobs[m.queueCursor]
			if job.Status == StatusPending || job.Status == StatusFailed || job.Status == StatusCancelled {
				curIdx := 0
				for i, p := range QualityPresets {
					if p.ID == job.Quality.ID {
						curIdx = i
						break
					}
				}
				if msg.String() == "left" || msg.String() == "h" {
					curIdx = (curIdx + len(QualityPresets) - 1) % len(QualityPresets)
				} else {
					curIdx = (curIdx + 1) % len(QualityPresets)
				}
				job.Quality = QualityPresets[curIdx]
				m.statusToast = fmt.Sprintf("Changed #%d quality to %s", job.ID, job.Quality.Name)
			}
			return m, nil
		}

	case "x", "delete":
		if m.dlFocus == focusQueue && len(m.jobs) > 0 && m.queueCursor < len(m.jobs) {
			job := m.jobs[m.queueCursor]
			if job.Status == StatusDownloading || job.Status == StatusQueued || job.Status == StatusFetching {
				if job.CancelFunc != nil {
					job.CancelFunc()
				}
				job.Status = StatusCancelled
				m.statusToast = fmt.Sprintf("Cancelled job #%d", job.ID)
			} else {
				// Remove from list
				m.jobs = append(m.jobs[:m.queueCursor], m.jobs[m.queueCursor+1:]...)
				if m.queueCursor >= len(m.jobs) && m.queueCursor > 0 {
					m.queueCursor--
				}
				m.statusToast = "Removed job from list"
			}
			return m, nil
		}

	case "p":
		if m.dlFocus != focusURL {
			if clipURL, ok := ReadClipboardYouTubeURL(); ok {
				m.urlInput.SetValue(clipURL)
				m.lastClipURL = clipURL
				m.addPendingJob(clipURL)
				m.clipBannerMsg = "Pasted & queued YouTube URL from clipboard!"
				m.metaLoading = true
				m.metaURL = clipURL
				m.metaTitle = ""
				m.metaErr = ""
				return m, FetchMetadataCmd(m.cfg.YtDlpPath, clipURL)
			}
			m.statusToast = "No valid YouTube URL found in clipboard."
			return m, nil
		}
	}

	if m.dlFocus == focusURL {
		prevVal := m.urlInput.Value()
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(msg)
		newVal := strings.TrimSpace(m.urlInput.Value())
		if newVal != strings.TrimSpace(prevVal) && IsYouTubeURL(newVal) && newVal != m.metaURL {
			m.metaLoading = true
			m.metaURL = newVal
			m.metaTitle = ""
			m.metaErr = ""
			return m, tea.Batch(cmd, FetchMetadataCmd(m.cfg.YtDlpPath, newVal))
		}
		return m, cmd
	}

	return m, nil
}

func (m model) updateSettingsTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		if msg.String() == "tab" {
			m.setFocus = (m.setFocus + 1) % 4
		} else {
			m.setFocus = (m.setFocus + 3) % 4
		}
		m.dirInput.Blur()
		m.ytdlpInput.Blur()
		if m.setFocus == settingsFocusSaveDir {
			m.dirInput.Focus()
		} else if m.setFocus == settingsFocusYtDlpPath {
			m.ytdlpInput.Focus()
		}
		return m, nil

	case "up", "k":
		if m.setFocus == settingsFocusQuality {
			if m.setQualCursor > 0 {
				m.setQualCursor--
			}
			return m, nil
		}

	case "down", "j":
		if m.setFocus == settingsFocusQuality {
			if m.setQualCursor < len(QualityPresets)-1 {
				m.setQualCursor++
			}
			return m, nil
		}

	case " ", "enter":
		if m.setFocus == settingsFocusClipboard && msg.String() == " " {
			m.cfg.AutoClipboard = !m.cfg.AutoClipboard
			return m, nil
		}
		if msg.String() == "enter" {
			m.saveSettings()
			m.switchTab(tabDownload)
			return m, nil
		}
	}

	if m.setFocus == settingsFocusSaveDir {
		var cmd tea.Cmd
		m.dirInput, cmd = m.dirInput.Update(msg)
		return m, cmd
	}
	if m.setFocus == settingsFocusYtDlpPath {
		var cmd tea.Cmd
		m.ytdlpInput, cmd = m.ytdlpInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#E11D48")).
			Padding(0, 2)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F43F5E")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#F43F5E")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94A3B8")).
				Padding(0, 2)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#475569")).
			Padding(0, 1)

	focusedPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#38BDF8")).
				Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#38BDF8"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10B981"))

	warnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F59E0B"))

	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#EF4444"))
)

func (m model) View() string {
	var b strings.Builder

	// Top Header & Tabs
	header := titleStyle.Render(" ▶ YT-TUI Downloader ")
	tab1 := inactiveTabStyle.Render("[F1] Download & Queue")
	tab2 := inactiveTabStyle.Render("[F2] Default Quality & Location")
	if m.tab == tabDownload {
		tab1 = activeTabStyle.Render("[F1] Download & Queue")
	} else {
		tab2 = activeTabStyle.Render("[F2] Default Quality & Location")
	}

	_, ytdlpOK := CheckYtDlpInstalled(m.cfg.YtDlpPath)
	var ytdlpBadge string
	if m.ytdlpChecking {
		if ytdlpOK {
			ytdlpBadge = warnStyle.Render("⟳ Updating yt-dlp...")
		} else {
			ytdlpBadge = warnStyle.Render("⟳ Auto-downloading yt-dlp...")
		}
	} else if ytdlpOK || m.ytdlpReady {
		verText := "● yt-dlp ready"
		if m.ytdlpVersion != "" {
			verText = fmt.Sprintf("● yt-dlp %s", m.ytdlpVersion)
		}
		ytdlpBadge = successStyle.Render(verText)
	} else {
		ytdlpBadge = errStyle.Render("✖ yt-dlp missing (Ctrl+U to retry download)")
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, header, "  ", tab1, " ", tab2, "   ", ytdlpBadge))
	b.WriteString("\n\n")

	panelW := m.width - 4
	if panelW < 60 {
		panelW = 60
	}

	if m.tab == tabDownload {
		b.WriteString(m. renderDownloadView(panelW))
	} else {
		b.WriteString(m.renderSettingsView(panelW))
	}

	// Footer Status & Shortcuts
	b.WriteString("\n")
	if m.statusToast != "" {
		b.WriteString("  " + warnStyle.Render("ℹ "+m.statusToast) + "\n")
	}
	help := subtleStyle.Render("  [Tab] Focus  [Enter/s] Start Selected  [Ctrl+A/a] Start All Ready  [←/→] Change Quality in Queue  [x] Cancel/Remove  [F2] Settings  [q] Quit")
	b.WriteString(help)

	return b.String()
}

func (m model) renderDownloadView(panelW int) string {
	var out strings.Builder

	// 1. URL Box
	urlBoxStyle := panelStyle.Copy().Width(panelW)
	if m.dlFocus == focusURL {
		urlBoxStyle = focusedPanelStyle.Copy().Width(panelW)
	}

	var urlContent strings.Builder
	clipStatus := subtleStyle.Render("Auto-Clipboard: OFF")
	if m.cfg.AutoClipboard {
		clipStatus = successStyle.Render("Auto-Clipboard: ON")
	}
	urlContent.WriteString(fmt.Sprintf("%s    %s\n", labelStyle.Render("1. YouTube Video URL"), clipStatus))
	urlContent.WriteString(m.urlInput.View() + "\n")

	if m.clipBannerMsg != "" {
		urlContent.WriteString(successStyle.Render("✓ "+m.clipBannerMsg) + "  ")
	}
	if m.metaLoading {
		urlContent.WriteString(warnStyle.Render("⟳ Fetching video metadata..."))
	} else if m.metaTitle != "" {
		info := fmt.Sprintf("🎬 %s", m.metaTitle)
		if m.metaChannel != "" {
			info += fmt.Sprintf("  •  👤 %s", m.metaChannel)
		}
		if m.metaDur != "" {
			info += fmt.Sprintf("  •  ⏱ %s", m.metaDur)
		}
		urlContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Bold(true).Render(info))
	} else if m.metaErr != "" {
		urlContent.WriteString(errStyle.Render("⚠ Metadata preview unavailable: " + m.metaErr))
	}

	out.WriteString(urlBoxStyle.Render(urlContent.String()) + "\n")

	// 2. Quality & Destination Box
	qualBoxStyle := panelStyle.Copy().Width(panelW)
	if m.dlFocus == focusQuality {
		qualBoxStyle = focusedPanelStyle.Copy().Width(panelW)
	}

	var qualContent strings.Builder
	qualContent.WriteString(fmt.Sprintf("%s   %s\n",
		labelStyle.Render("2. Quality Preset (↑/↓ to change, Enter to Download)"),
		subtleStyle.Render("Save to: "+m.cfg.DownloadDir),
	))

	for i, p := range QualityPresets {
		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))
		if i == m.qualityCursor {
			cursor = "▸ "
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8"))
		}
		defaultTag := ""
		if p.ID == m.cfg.DefaultQuality {
			defaultTag = subtleStyle.Render(" [Default]")
		}
		qualContent.WriteString(fmt.Sprintf("%s%-34s %s%s\n",
			cursor,
			nameStyle.Render(p.Name),
			subtleStyle.Render("- "+p.Description),
			defaultTag,
		))
	}
	out.WriteString(qualBoxStyle.Render(strings.TrimSuffix(qualContent.String(), "\n")) + "\n")

	// 3. Downloads Queue Box
	queueBoxStyle := panelStyle.Copy().Width(panelW)
	if m.dlFocus == focusQueue {
		queueBoxStyle = focusedPanelStyle.Copy().Width(panelW)
	}

	var qContent strings.Builder
	qContent.WriteString(fmt.Sprintf("%s %s\n",
		labelStyle.Render(fmt.Sprintf("3. Downloads Queue (%d)", len(m.jobs))),
		subtleStyle.Render("(Enter/'s': Start item • 'a'/Ctrl+A: Start all • ←/→: Change quality • 'x': Cancel/Remove)"),
	))

	if len(m.jobs) == 0 {
		qContent.WriteString(subtleStyle.Render("  No links in queue yet. Copy any YouTube link and it will appear here automatically!"))
	} else {
		maxVisible := 6
		for i, job := range m.jobs {
			if i >= maxVisible {
				qContent.WriteString(subtleStyle.Render(fmt.Sprintf("  ... and %d more downloads", len(m.jobs)-maxVisible)))
				break
			}
			sel := "  "
			if m.dlFocus == focusQueue && i == m.queueCursor {
				sel = "▸ "
			}

			statusBadge := m.formatStatusBadge(job.Status)
			title := job.Title
			if len(title) > 48 {
				title = title[:45] + "..."
			}

			qContent.WriteString(fmt.Sprintf("%s#%d [%s] %s  %s\n",
				sel, job.ID, job.Quality.ID, title, statusBadge))

			if job.Status == StatusDownloading || job.Status == StatusMerging {
				bar := m.progBar.ViewAs(job.Percent / 100.0)
				stats := fmt.Sprintf(" %.1f%%", job.Percent)
				if job.Size != "" {
					stats += fmt.Sprintf(" of %s", job.Size)
				}
				if job.Speed != "" {
					stats += fmt.Sprintf(" • %s", job.Speed)
				}
				if job.ETA != "" {
					stats += fmt.Sprintf(" • ETA %s", job.ETA)
				}
				qContent.WriteString(fmt.Sprintf("     %s%s\n", bar, subtleStyle.Render(stats)))
			} else if job.Status == StatusCompleted && job.OutputFile != "" {
				qContent.WriteString("     " + subtleStyle.Render("Saved: "+job.OutputFile) + "\n")
			} else if job.Status == StatusFailed && job.ErrorMsg != "" {
				qContent.WriteString("     " + errStyle.Render("Error: "+job.ErrorMsg) + "\n")
			}
		}
	}

	out.WriteString(queueBoxStyle.Render(strings.TrimSuffix(qContent.String(), "\n")))
	return out.String()
}

func (m model) renderSettingsView(panelW int) string {
	var out strings.Builder

	// 1. Default Quality
	qBox := panelStyle.Copy().Width(panelW)
	if m.setFocus == settingsFocusQuality {
		qBox = focusedPanelStyle.Copy().Width(panelW)
	}
	var qc strings.Builder
	qc.WriteString(labelStyle.Render("1. Default Video/Audio Quality (↑/↓ to select)") + "\n")
	for i, p := range QualityPresets {
		radio := "( )"
		st := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))
		if i == m.setQualCursor {
			radio = "(•)"
			st = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8"))
		}
		qc.WriteString(fmt.Sprintf("  %s %-34s %s\n", radio, st.Render(p.Name), subtleStyle.Render(p.Description)))
	}
	out.WriteString(qBox.Render(strings.TrimSuffix(qc.String(), "\n")) + "\n")

	// 2. Default Save Location
	dirBox := panelStyle.Copy().Width(panelW)
	if m.setFocus == settingsFocusSaveDir {
		dirBox = focusedPanelStyle.Copy().Width(panelW)
	}
	var dc strings.Builder
	dc.WriteString(labelStyle.Render("2. Default Save Location (Folder Path)") + "\n")
	dc.WriteString(m.dirInput.View() + "\n")
	dirVal := strings.TrimSpace(m.dirInput.Value())
	if info, err := os.Stat(dirVal); err == nil && info.IsDir() {
		dc.WriteString(successStyle.Render("✓ Directory exists"))
	} else {
		dc.WriteString(warnStyle.Render("ℹ Directory will be created automatically when downloading"))
	}
	out.WriteString(dirBox.Render(dc.String()) + "\n")

	// 3. Clipboard Auto-Detection Toggle
	clipBox := panelStyle.Copy().Width(panelW)
	if m.setFocus == settingsFocusClipboard {
		clipBox = focusedPanelStyle.Copy().Width(panelW)
	}
	chk := "[ ]"
	if m.cfg.AutoClipboard {
		chk = "[✓]"
	}
	clipStr := fmt.Sprintf("%s\n  %s Automatically read clipboard when a YouTube URL is copied (Press Space to toggle)",
		labelStyle.Render("3. Clipboard Monitoring"),
		successStyle.Render(chk),
	)
	out.WriteString(clipBox.Render(clipStr) + "\n")

	// 4. yt-dlp Executable Path
	ytBox := panelStyle.Copy().Width(panelW)
	if m.setFocus == settingsFocusYtDlpPath {
		ytBox = focusedPanelStyle.Copy().Width(panelW)
	}
	var yc strings.Builder
	yc.WriteString(labelStyle.Render("4. yt-dlp Executable Path (Press Ctrl+U anytime to auto-download/update)") + "\n")
	yc.WriteString(m.ytdlpInput.View() + "\n")
	if m.ytdlpChecking {
		yc.WriteString(warnStyle.Render("⟳ Downloading / updating yt-dlp in the background..."))
	} else if resolved, ok := CheckYtDlpInstalled(strings.TrimSpace(m.ytdlpInput.Value())); ok {
		verInfo := ""
		if m.ytdlpVersion != "" {
			verInfo = " (version " + m.ytdlpVersion + ")"
		}
		yc.WriteString(successStyle.Render("✓ Found: " + resolved + verInfo))
	} else {
		yc.WriteString(errStyle.Render("✖ Not found. Press [Ctrl+U] to auto-download latest yt-dlp binary"))
	}
	out.WriteString(ytBox.Render(yc.String()) + "\n")

	out.WriteString("\n  " + successStyle.Render("Press [Enter] or [Ctrl+S] to save settings and return to Download view."))
	return out.String()
}

func (m model) formatStatusBadge(s JobStatus) string {
	switch s {
	case StatusPending:
		return warnStyle.Render("⏸ Ready (Press Enter/'s' to start)")
	case StatusCompleted:
		return successStyle.Render("✓ Completed")
	case StatusDownloading:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8")).Render("↓ Downloading")
	case StatusMerging:
		return warnStyle.Render("⚙ Processing/Merging")
	case StatusFetching:
		return warnStyle.Render("⟳ Fetching Info")
	case StatusQueued:
		return subtleStyle.Render("• Queued")
	case StatusFailed:
		return errStyle.Render("✖ Failed")
	case StatusCancelled:
		return subtleStyle.Render("⊘ Cancelled")
	default:
		return string(s)
	}
}

