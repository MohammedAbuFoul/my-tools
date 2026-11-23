// NetCheck (Go) — Lightweight native Windows floating internet status widget.
//
// Zero external dependencies. Uses native Win32 layered window (per-pixel alpha)
// with mathematically anti-aliased SDF rendering, native Segoe UI text,
// and Windows iphlpapi.dll ICMP echo (no subprocesses, no console window).
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Win32 DLLs & Procedures
// ---------------------------------------------------------------------------
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")

	procSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")
	procSetProcessDPIAware     = user32.NewProc("SetProcessDPIAware")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procUpdateLayeredWindow  = user32.NewProc("UpdateLayeredWindow")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procSetCursor            = user32.NewProc("SetCursor")
	procReleaseCapture       = user32.NewProc("ReleaseCapture")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procTrackMouseEvent      = user32.NewProc("TrackMouseEvent")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procAppendMenuW          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	procDestroyMenu          = user32.NewProc("DestroyMenu")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procDrawTextW            = user32.NewProc("DrawTextW")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procSetBkMode          = gdi32.NewProc("SetBkMode")

	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")

	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

// ---------------------------------------------------------------------------
// Win32 Constants & Structs
// ---------------------------------------------------------------------------
const (
	WS_POPUP         = 0x80000000
	WS_VISIBLE       = 0x10000000
	WS_EX_TOPMOST    = 0x00000008
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_LAYERED    = 0x00080000

	SW_SHOW = 5

	HWND_TOPMOST   = ^uintptr(0) // -1
	HWND_NOTOPMOST = ^uintptr(1) // -2

	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010

	WM_DESTROY       = 0x0002
	WM_LBUTTONDOWN   = 0x0201
	WM_RBUTTONUP     = 0x0205
	WM_MOUSEMOVE     = 0x0200
	WM_MOUSELEAVE    = 0x02A3
	WM_NCLBUTTONDOWN = 0x00A1
	WM_SETCURSOR     = 0x0020
	WM_APP_PING      = 0x8001

	HTCAPTION = 2

	IDC_ARROW = 32512
	IDC_HAND  = 32649

	TME_LEAVE = 0x00000002

	SPI_GETWORKAREA = 0x0030

	ULW_ALPHA    = 0x00000002
	AC_SRC_OVER  = 0x00
	AC_SRC_ALPHA = 0x01
	BI_RGB       = 0
	DIB_RGB_COLS = 0

	TRANSPARENT         = 1
	ANTIALIASED_QUALITY = 4
	FW_NORMAL           = 400
	FW_SEMIBOLD         = 600
	DT_LEFT             = 0x00000000
	DT_SINGLELINE       = 0x00000020
	DT_VCENTER          = 0x00000004
	DT_NOPREFIX         = 0x00000800

	MF_STRING    = 0x0000
	MF_SEPARATOR = 0x0800
	MF_CHECKED   = 0x0008
	MF_UNCHECKED = 0x0000
	TPM_RIGHTBTN = 0x0002
	TPM_RETURN   = 0x0100

	CMD_TOPMOST    = 1001
	CMD_PING_NOW   = 1002
	CMD_SNAP_TL    = 1003
	CMD_SNAP_TR    = 1004
	CMD_SNAP_BL    = 1005
	CMD_SNAP_BR    = 1006
	CMD_EXIT       = 1007
	CMD_RELOAD_ENV = 1008

	SM_XVIRTUALSCREEN  = 76
	SM_YVIRTUALSCREEN  = 77
	SM_CXVIRTUALSCREEN = 78
	SM_CYVIRTUALSCREEN = 79
)

type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type POINT struct {
	X, Y int32
}

type SIZE struct {
	CX, CY int32
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

type TRACKMOUSEEVENT struct {
	Size      uint32
	Flags     uint32
	HwndTrack uintptr
	HoverTime uint32
}

// ---------------------------------------------------------------------------
// Widget Layout & State
// ---------------------------------------------------------------------------
const (
	defaultTargetHost          = "8.8.8.8"
	defaultPingIntervalOnline  = 5
	defaultPingIntervalOffline = 1
	defaultPingTimeoutOnline   = 2000
	defaultPingTimeoutOffline  = 900
	shadowPad                  = 10
	pillW                      = 230
	pillH                      = 54
	winW                       = pillW + shadowPad*2
	winH                       = pillH + shadowPad*2
	pillRadius                 = 16.0
	screenMargin               = 16
)

const (
	stateChecking = 0
	stateOnline   = 1
	stateOffline  = 2
)

type Config struct {
	TargetHost             string
	PingIntervalOnlineSec  int
	PingIntervalOfflineSec int
	PingTimeoutOnlineMs    int
	PingTimeoutOfflineMs   int
	AlwaysOnTop            bool
	HasPos                 bool
	PosX                   int32
	PosY                   int32
}

func defaultConfig() Config {
	return Config{
		TargetHost:             defaultTargetHost,
		PingIntervalOnlineSec:  defaultPingIntervalOnline,
		PingIntervalOfflineSec: defaultPingIntervalOffline,
		PingTimeoutOnlineMs:    defaultPingTimeoutOnline,
		PingTimeoutOfflineMs:   defaultPingTimeoutOffline,
		AlwaysOnTop:            true,
	}
}

func getEnvFilePath() string {
	exe, err := os.Executable()
	if err == nil && exe != "" {
		return filepath.Join(filepath.Dir(exe), ".env")
	}
	return ".env"
}

func loadConfig(path string) (Config, bool) {
	cfg := defaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return cfg, false
	}
	defer f.Close()

	var hasX, hasY bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		switch strings.ToUpper(key) {
		case "TARGET_HOST":
			if val != "" {
				cfg.TargetHost = val
			}
		case "PING_INTERVAL_ONLINE_SEC":
			if n, err := strconv.Atoi(val); err == nil && n >= 1 {
				cfg.PingIntervalOnlineSec = n
			}
		case "PING_INTERVAL_OFFLINE_SEC":
			if n, err := strconv.Atoi(val); err == nil && n >= 1 {
				cfg.PingIntervalOfflineSec = n
			}
		case "PING_TIMEOUT_ONLINE_MS":
			if n, err := strconv.Atoi(val); err == nil && n >= 100 {
				cfg.PingTimeoutOnlineMs = n
			}
		case "PING_TIMEOUT_OFFLINE_MS":
			if n, err := strconv.Atoi(val); err == nil && n >= 100 {
				cfg.PingTimeoutOfflineMs = n
			}
		case "ALWAYS_ON_TOP":
			if b, err := strconv.ParseBool(strings.ToLower(val)); err == nil {
				cfg.AlwaysOnTop = b
			}
		case "POS_X":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.PosX = int32(n)
				hasX = true
			}
		case "POS_Y":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.PosY = int32(n)
				hasY = true
			}
		}
	}
	cfg.HasPos = hasX && hasY
	return cfg, true
}

func saveConfig(path string, cfg Config) {
	var b strings.Builder
	b.WriteString("# NetCheck settings (edit anytime and right-click -> Reload .env)\n")
	fmt.Fprintf(&b, "TARGET_HOST=%s\n", cfg.TargetHost)
	fmt.Fprintf(&b, "PING_INTERVAL_ONLINE_SEC=%d\n", cfg.PingIntervalOnlineSec)
	fmt.Fprintf(&b, "PING_INTERVAL_OFFLINE_SEC=%d\n", cfg.PingIntervalOfflineSec)
	fmt.Fprintf(&b, "PING_TIMEOUT_ONLINE_MS=%d\n", cfg.PingTimeoutOnlineMs)
	fmt.Fprintf(&b, "PING_TIMEOUT_OFFLINE_MS=%d\n", cfg.PingTimeoutOfflineMs)
	fmt.Fprintf(&b, "ALWAYS_ON_TOP=%t\n", cfg.AlwaysOnTop)
	if cfg.HasPos {
		fmt.Fprintf(&b, "POS_X=%d\n", cfg.PosX)
		fmt.Fprintf(&b, "POS_Y=%d\n", cfg.PosY)
	} else {
		b.WriteString("POS_X=\n")
		b.WriteString("POS_Y=\n")
	}
	_ = os.WriteFile(path, []byte(b.String()), 0644)
}

type RectF struct {
	X1, Y1, X2, Y2 float64
}

func (r RectF) Contains(x, y float64) bool {
	return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2
}

var (
	pinBtn   = RectF{float64(shadowPad + pillW - 64), float64(shadowPad + 12), float64(shadowPad + pillW - 36), float64(shadowPad + 42)}
	closeBtn = RectF{float64(shadowPad + pillW - 34), float64(shadowPad + 12), float64(shadowPad + pillW - 10), float64(shadowPad + 42)}
)

type App struct {
	hwnd        uintptr
	cfgMu       sync.RWMutex
	cfg         Config
	envPath     string
	alwaysOnTop bool
	status      int
	rttMs       int
	hoverPin    bool
	hoverClose  bool
	tracking    bool
	fontTitle   uintptr
	fontSub     uintptr
	pingNowCh   chan struct{}
	stopCh      chan struct{}
}

func (a *App) getConfig() Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

func (a *App) persistCurrentState() {
	var r RECT
	hasRect := false
	if a.hwnd != 0 {
		ret, _, _ := procGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
		hasRect = ret != 0
	}
	a.cfgMu.Lock()
	a.cfg.AlwaysOnTop = a.alwaysOnTop
	if hasRect {
		a.cfg.HasPos = true
		a.cfg.PosX = r.Left
		a.cfg.PosY = r.Top
	}
	cfgCopy := a.cfg
	a.cfgMu.Unlock()
	saveConfig(a.envPath, cfgCopy)
}

var app *App

// ---------------------------------------------------------------------------
// Native Windows ICMP Ping (iphlpapi.dll)
// ---------------------------------------------------------------------------
func resolveIPv4(host string) (uint32, bool) {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return binary.LittleEndian.Uint32(ip4), true
		}
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return 0, false
	}
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			return binary.LittleEndian.Uint32(ip4), true
		}
	}
	return 0, false
}

func pingHost(host string, timeoutMs uint32) (bool, int) {
	destIP, ok := resolveIPv4(host)
	if !ok {
		return false, 0
	}

	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 || h == ^uintptr(0) {
		return false, 0
	}
	defer procIcmpCloseHandle.Call(h)

	payload := []byte("netcheck-ping-payload-0123456789")
	replyBuf := make([]byte, 256)

	count, _, _ := procIcmpSendEcho.Call(
		h,
		uintptr(destIP),
		uintptr(unsafe.Pointer(&payload[0])),
		uintptr(len(payload)),
		0,
		uintptr(unsafe.Pointer(&replyBuf[0])),
		uintptr(len(replyBuf)),
		uintptr(timeoutMs),
	)
	if count > 0 {
		status := binary.LittleEndian.Uint32(replyBuf[4:8])
		rtt := binary.LittleEndian.Uint32(replyBuf[8:12])
		if status == 0 {
			return true, int(rtt)
		}
	}
	return false, 0
}

// ---------------------------------------------------------------------------
// Anti-Aliased Software Canvas (Premultiplied BGRA) + Native Segoe UI Text
// ---------------------------------------------------------------------------
type rgba struct {
	r, g, b, a float64
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func smoothstep(edge0, edge1, x float64) float64 {
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3 - 2*t)
}

func sdRoundedRect(px, py, cx, cy, hx, hy, r float64) float64 {
	qx := math.Abs(px-cx) - (hx - r)
	qy := math.Abs(py-cy) - (hy - r)
	ox := math.Max(qx, 0)
	oy := math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func sdSegment(px, py, ax, ay, bx, by float64) float64 {
	pax, pay := px-ax, py-ay
	bax, bay := bx-ax, by-ay
	denom := bax*bax + bay*bay
	if denom == 0 {
		return math.Hypot(pax, pay)
	}
	h := clamp01((pax*bax + pay*bay) / denom)
	return math.Hypot(pax-bax*h, pay-bay*h)
}

func blendOver(dst []byte, idx int, src rgba, coverage float64) {
	sa := src.a * clamp01(coverage)
	if sa <= 0.0005 {
		return
	}
	// dst is premultiplied BGRA [0..255]
	db := float64(dst[idx+0]) / 255.0
	dg := float64(dst[idx+1]) / 255.0
	dr := float64(dst[idx+2]) / 255.0
	da := float64(dst[idx+3]) / 255.0

	inv := 1.0 - sa
	or := src.r*sa + dr*inv
	og := src.g*sa + dg*inv
	ob := src.b*sa + db*inv
	oa := sa + da*inv

	dst[idx+0] = byte(clamp01(ob) * 255.0)
	dst[idx+1] = byte(clamp01(og) * 255.0)
	dst[idx+2] = byte(clamp01(or) * 255.0)
	dst[idx+3] = byte(clamp01(oa) * 255.0)
}

func hexColor(hex uint32, alpha float64) rgba {
	return rgba{
		r: float64((hex>>16)&0xFF) / 255.0,
		g: float64((hex>>8)&0xFF) / 255.0,
		b: float64(hex&0xFF) / 255.0,
		a: alpha,
	}
}

func createDIB(hdc uintptr, w, h int) (uintptr, []byte) {
	var bi BITMAPINFO
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	bi.Header.Height = -int32(h) // top-down
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = BI_RGB

	var bitsPtr unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&bi)),
		DIB_RGB_COLS,
		uintptr(unsafe.Pointer(&bitsPtr)),
		0, 0,
	)
	if hbmp == 0 || bitsPtr == nil {
		return 0, nil
	}
	slice := unsafe.Slice((*byte)(bitsPtr), w*h*4)
	return hbmp, slice
}

func drawTextMask(dst []byte, w, h int, font uintptr, text string, rect RECT, col rgba) {
	screenDC, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	defer procDeleteDC.Call(memDC)

	hbmp, mask := createDIB(memDC, w, h)
	if hbmp == 0 {
		return
	}
	defer procDeleteObject.Call(hbmp)

	oldBmp, _, _ := procSelectObject.Call(memDC, hbmp)
	defer procSelectObject.Call(memDC, oldBmp)

	oldFont, _, _ := procSelectObject.Call(memDC, font)
	defer procSelectObject.Call(memDC, oldFont)

	procSetBkMode.Call(memDC, TRANSPARENT)
	procSetTextColor.Call(memDC, 0x00FFFFFF) // white text on black background

	u16, _ := syscall.UTF16FromString(text)
	procDrawTextW.Call(
		memDC,
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)-1),
		uintptr(unsafe.Pointer(&rect)),
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX,
	)

	// Composite antialiased luminance from mask onto dst
	yStart := int(rect.Top)
	if yStart < 0 {
		yStart = 0
	}
	yEnd := int(rect.Bottom)
	if yEnd > h {
		yEnd = h
	}
	xStart := int(rect.Left)
	if xStart < 0 {
		xStart = 0
	}
	xEnd := int(rect.Right)
	if xEnd > w {
		xEnd = w
	}

	for y := yStart; y < yEnd; y++ {
		row := y * w * 4
		for x := xStart; x < xEnd; x++ {
			idx := row + x*4
			lum := float64(mask[idx+1]) / 255.0
			if lum > 0.01 {
				blendOver(dst, idx, col, lum)
			}
		}
	}
}

func (a *App) render() {
	screenDC, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	defer procDeleteDC.Call(memDC)

	hbmp, pixels := createDIB(memDC, winW, winH)
	if hbmp == 0 {
		return
	}
	defer procDeleteObject.Call(hbmp)

	oldBmp, _, _ := procSelectObject.Call(memDC, hbmp)
	defer procSelectObject.Call(memDC, oldBmp)

	// Colors
	bgCol := hexColor(0x161922, 0.95)
	borderCol := hexColor(0x323A50, 0.95)
	shadowCol := rgba{0, 0, 0, 0.42}

	host := a.getConfig().TargetHost
	var dotCol rgba
	var title, subtitle string
	switch a.status {
	case stateOnline:
		dotCol = hexColor(0x22C55E, 1.0)
		title = "Online"
		subtitle = fmt.Sprintf("%s · %d ms", host, a.rttMs)
	case stateOffline:
		dotCol = hexColor(0xEF4444, 1.0)
		title = "Offline"
		subtitle = fmt.Sprintf("%s · No reply", host)
	default:
		dotCol = hexColor(0xF59E0B, 1.0)
		title = "Checking..."
		subtitle = fmt.Sprintf("Pinging %s", host)
	}

	// Geometry centers
	pillCX := float64(winW) / 2.0
	pillCY := float64(winH) / 2.0
	pillHX := float64(pillW) / 2.0
	pillHY := float64(pillH) / 2.0

	dotCX := float64(shadowPad + 24)
	dotCY := float64(shadowPad + pillH/2)

	pinCX := (pinBtn.X1 + pinBtn.X2) / 2.0
	pinCY := (pinBtn.Y1 + pinBtn.Y2) / 2.0
	closeCX := (closeBtn.X1 + closeBtn.X2) / 2.0
	closeCY := (closeBtn.Y1 + closeBtn.Y2) / 2.0

	pinBg := rgba{}
	pinFg := hexColor(0x64748B, 1.0)
	if a.alwaysOnTop {
		pinBg = hexColor(0x1E3A5F, 0.95)
		pinFg = hexColor(0x60A5FA, 1.0)
	} else if a.hoverPin {
		pinBg = hexColor(0x262D40, 0.95)
		pinFg = hexColor(0xF1F5F9, 1.0)
	}

	closeBg := rgba{}
	closeFg := hexColor(0x64748B, 1.0)
	if a.hoverClose {
		closeBg = hexColor(0x4C1D24, 0.95)
		closeFg = hexColor(0xFCA5A5, 1.0)
	}

	// Per-pixel SDF rendering
	for y := 0; y < winH; y++ {
		py := float64(y) + 0.5
		row := y * winW * 4
		for x := 0; x < winW; x++ {
			px := float64(x) + 0.5
			idx := row + x*4

			// 1. Soft drop shadow (offset +2.5px Y)
			dShadow := sdRoundedRect(px, py-2.5, pillCX, pillCY, pillHX-1, pillHY-1, pillRadius)
			if dShadow < 9.0 {
				shAlpha := 1.0 - smoothstep(-1.0, 9.0, dShadow)
				blendOver(pixels, idx, shadowCol, shAlpha*shAlpha)
			}

			// 2. Pill body + crisp 1px border
			dPill := sdRoundedRect(px, py, pillCX, pillCY, pillHX, pillHY, pillRadius)
			if dPill < 1.0 {
				fillCov := clamp01(0.5 - dPill)
				blendOver(pixels, idx, bgCol, fillCov)

				// 1px inner border
				borderDist := math.Abs(dPill + 0.5)
				if borderDist < 1.0 {
					blendOver(pixels, idx, borderCol, clamp01(1.0-borderDist)*fillCov)
				}
			}

			// 3. Status dot + soft radial glow
			dDot := math.Hypot(px-dotCX, py-dotCY)
			if dDot < 13.0 {
				glow := 1.0 - smoothstep(4.5, 13.0, dDot)
				blendOver(pixels, idx, rgba{dotCol.r, dotCol.g, dotCol.b, 0.28}, glow)
				if dDot < 6.0 {
					blendOver(pixels, idx, dotCol, clamp01(5.2-dDot))
				}
			}

			// 4. Pin button circle + vector pushpin icon
			dPinCircle := math.Hypot(px-pinCX, py-pinCY)
			if pinBg.a > 0 && dPinCircle < 14.0 {
				blendOver(pixels, idx, pinBg, clamp01(13.0-dPinCircle))
			}
			if dPinCircle < 12.0 {
				dTop := sdSegment(px, py, pinCX-3.5, pinCY-4.8, pinCX+3.5, pinCY-4.8)
				dStem := sdSegment(px, py, pinCX, pinCY-4.8, pinCX, pinCY+0.8)
				dBase := sdSegment(px, py, pinCX-4.8, pinCY+0.8, pinCX+4.8, pinCY+0.8)
				dNeedle := sdSegment(px, py, pinCX, pinCY+0.8, pinCX, pinCY+5.8)
				dIcon := math.Min(math.Min(dTop, dStem), dBase)
				cov := clamp01(1.1 - dIcon)
				covNeedle := clamp01(0.85 - dNeedle)
				if covNeedle > cov {
					cov = covNeedle
				}
				if cov > 0 {
					blendOver(pixels, idx, pinFg, cov)
				}
			}

			// 5. Close button circle + vector X icon
			dCloseCircle := math.Hypot(px-closeCX, py-closeCY)
			if closeBg.a > 0 && dCloseCircle < 13.0 {
				blendOver(pixels, idx, closeBg, clamp01(12.0-dCloseCircle))
			}
			if dCloseCircle < 10.0 {
				d1 := sdSegment(px, py, closeCX-3.8, closeCY-3.8, closeCX+3.8, closeCY+3.8)
				d2 := sdSegment(px, py, closeCX+3.8, closeCY-3.8, closeCX-3.8, closeCY+3.8)
				cov := clamp01(1.05 - math.Min(d1, d2))
				if cov > 0 {
					blendOver(pixels, idx, closeFg, cov)
				}
			}
		}
	}

	// Render crisp Segoe UI text via antialiased alpha mask
	drawTextMask(
		pixels, winW, winH, a.fontTitle, title,
		RECT{
			Left:   shadowPad + 44,
			Top:    shadowPad + 8,
			Right:  shadowPad + pillW - 68,
			Bottom: shadowPad + 29,
		},
		hexColor(0xF1F5F9, 1.0),
	)
	drawTextMask(
		pixels, winW, winH, a.fontSub, subtitle,
		RECT{
			Left:   shadowPad + 44,
			Top:    shadowPad + 27,
			Right:  shadowPad + pillW - 68,
			Bottom: shadowPad + 46,
		},
		hexColor(0x8B95A8, 1.0),
	)

	// Push premultiplied bitmap to layered window
	ptSrc := POINT{0, 0}
	sz := SIZE{winW, winH}
	blend := BLENDFUNCTION{
		BlendOp:             AC_SRC_OVER,
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         AC_SRC_ALPHA,
	}
	procUpdateLayeredWindow.Call(
		a.hwnd,
		screenDC,
		0,
		uintptr(unsafe.Pointer(&sz)),
		memDC,
		uintptr(unsafe.Pointer(&ptSrc)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ULW_ALPHA,
	)
}

// ---------------------------------------------------------------------------
// Positioning, Topmost, Context Menu
// ---------------------------------------------------------------------------
func getWorkArea() RECT {
	var r RECT
	ret, _, _ := procSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ret == 0 {
		return RECT{0, 0, 1920, 1040}
	}
	return r
}

func isPosVisible(x, y int32) bool {
	vx, _, _ := procGetSystemMetrics.Call(SM_XVIRTUALSCREEN)
	vy, _, _ := procGetSystemMetrics.Call(SM_YVIRTUALSCREEN)
	vw, _, _ := procGetSystemMetrics.Call(SM_CXVIRTUALSCREEN)
	vh, _, _ := procGetSystemMetrics.Call(SM_CYVIRTUALSCREEN)
	left := int32(vx)
	top := int32(vy)
	right := left + int32(vw)
	bottom := top + int32(vh)
	return x+winW > left+20 && x < right-20 && y+winH > top+20 && y < bottom-20
}

func (a *App) moveTo(x, y int32) {
	zOrder := HWND_NOTOPMOST
	if a.alwaysOnTop {
		zOrder = HWND_TOPMOST
	}
	procSetWindowPos.Call(a.hwnd, zOrder, uintptr(x), uintptr(y), 0, 0, SWP_NOSIZE|SWP_NOACTIVATE)
}

func (a *App) snapToCorner(corner string) {
	wa := getWorkArea()
	var x, y int32
	switch corner {
	case "tl":
		x = wa.Left + screenMargin - shadowPad
		y = wa.Top + screenMargin - shadowPad
	case "tr":
		x = wa.Right - pillW - screenMargin - shadowPad
		y = wa.Top + screenMargin - shadowPad
	case "bl":
		x = wa.Left + screenMargin - shadowPad
		y = wa.Bottom - pillH - screenMargin - shadowPad
	default: // "br"
		x = wa.Right - pillW - screenMargin - shadowPad
		y = wa.Bottom - pillH - screenMargin - shadowPad
	}
	a.moveTo(x, y)
	a.persistCurrentState()
}

func (a *App) applyTopmost() {
	zOrder := HWND_NOTOPMOST
	if a.alwaysOnTop {
		zOrder = HWND_TOPMOST
	}
	procSetWindowPos.Call(a.hwnd, zOrder, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	a.persistCurrentState()
	a.render()
}

func (a *App) reloadEnv() {
	cfg, ok := loadConfig(a.envPath)
	if !ok {
		return
	}
	a.cfgMu.Lock()
	a.cfg = cfg
	a.cfgMu.Unlock()

	a.alwaysOnTop = cfg.AlwaysOnTop
	if cfg.HasPos && isPosVisible(cfg.PosX, cfg.PosY) {
		a.moveTo(cfg.PosX, cfg.PosY)
	}
	zOrder := HWND_NOTOPMOST
	if a.alwaysOnTop {
		zOrder = HWND_TOPMOST
	}
	procSetWindowPos.Call(a.hwnd, zOrder, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	a.render()
	select {
	case a.pingNowCh <- struct{}{}:
	default:
	}
}

func (a *App) showContextMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	addItem := func(flags uintptr, id uintptr, label string) {
		var p *uint16
		if label != "" {
			p, _ = syscall.UTF16PtrFromString(label)
		}
		procAppendMenuW.Call(hMenu, flags, id, uintptr(unsafe.Pointer(p)))
	}

	topFlag := uintptr(MF_STRING | MF_UNCHECKED)
	if a.alwaysOnTop {
		topFlag = MF_STRING | MF_CHECKED
	}
	addItem(topFlag, CMD_TOPMOST, "Always on Top")
	addItem(MF_STRING, CMD_PING_NOW, "Ping Now")
	addItem(MF_STRING, CMD_RELOAD_ENV, "Reload .env")
	addItem(MF_SEPARATOR, 0, "")
	addItem(MF_STRING, CMD_SNAP_TL, "Snap: Top-Left")
	addItem(MF_STRING, CMD_SNAP_TR, "Snap: Top-Right")
	addItem(MF_STRING, CMD_SNAP_BL, "Snap: Bottom-Left")
	addItem(MF_STRING, CMD_SNAP_BR, "Snap: Bottom-Right")
	addItem(MF_SEPARATOR, 0, "")
	addItem(MF_STRING, CMD_EXIT, "Exit")

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(a.hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_RIGHTBTN|TPM_RETURN,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		a.hwnd,
		0,
	)

	switch cmd {
	case CMD_TOPMOST:
		a.alwaysOnTop = !a.alwaysOnTop
		a.applyTopmost()
	case CMD_PING_NOW:
		select {
		case a.pingNowCh <- struct{}{}:
		default:
		}
	case CMD_RELOAD_ENV:
		a.reloadEnv()
	case CMD_SNAP_TL:
		a.snapToCorner("tl")
	case CMD_SNAP_TR:
		a.snapToCorner("tr")
	case CMD_SNAP_BL:
		a.snapToCorner("bl")
	case CMD_SNAP_BR:
		a.snapToCorner("br")
	case CMD_EXIT:
		procDestroyWindow.Call(a.hwnd)
	}
}

// ---------------------------------------------------------------------------
// Window Procedure
// ---------------------------------------------------------------------------
func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_APP_PING:
		if wParam == 1 {
			app.status = stateOnline
			app.rttMs = int(lParam)
		} else {
			app.status = stateOffline
			app.rttMs = 0
		}
		app.render()
		return 0

	case WM_MOUSEMOVE:
		x := float64(int16(lParam & 0xFFFF))
		y := float64(int16((lParam >> 16) & 0xFFFF))
		if !app.tracking {
			tme := TRACKMOUSEEVENT{
				Size:      uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})),
				Flags:     TME_LEAVE,
				HwndTrack: hwnd,
			}
			procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			app.tracking = true
		}
		inPin := pinBtn.Contains(x, y)
		inClose := closeBtn.Contains(x, y)
		if inPin != app.hoverPin || inClose != app.hoverClose {
			app.hoverPin = inPin
			app.hoverClose = inClose
			app.render()
		}
		return 0

	case WM_SETCURSOR:
		cursorID := uintptr(IDC_ARROW)
		if app.hoverPin || app.hoverClose {
			cursorID = IDC_HAND
		}
		hCur, _, _ := procLoadCursorW.Call(0, cursorID)
		procSetCursor.Call(hCur)
		return 1

	case WM_MOUSELEAVE:
		app.tracking = false
		if app.hoverPin || app.hoverClose {
			app.hoverPin = false
			app.hoverClose = false
			app.render()
		}
		return 0

	case WM_LBUTTONDOWN:
		x := float64(int16(lParam & 0xFFFF))
		y := float64(int16((lParam >> 16) & 0xFFFF))
		if pinBtn.Contains(x, y) {
			app.alwaysOnTop = !app.alwaysOnTop
			app.applyTopmost()
			return 0
		}
		if closeBtn.Contains(x, y) {
			procDestroyWindow.Call(hwnd)
			return 0
		}
		// Smooth native OS window drag
		procReleaseCapture.Call()
		procSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		app.persistCurrentState()
		return 0

	case WM_RBUTTONUP:
		app.showContextMenu()
		return 0

	case WM_DESTROY:
		app.persistCurrentState()
		select {
		case <-app.stopCh:
		default:
			close(app.stopCh)
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// ---------------------------------------------------------------------------
// Ping Worker & Entrypoint
// ---------------------------------------------------------------------------
func (a *App) pingLoop() {
	lastOnline := true
	doPing := func(cfg Config) bool {
		timeout := uint32(cfg.PingTimeoutOnlineMs)
		if !lastOnline {
			timeout = uint32(cfg.PingTimeoutOfflineMs)
		}
		online, rtt := pingHost(cfg.TargetHost, timeout)
		lastOnline = online
		var wp uintptr
		if online {
			wp = 1
		}
		procPostMessageW.Call(a.hwnd, WM_APP_PING, wp, uintptr(rtt))
		return online
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-a.pingNowCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}

		cfg := a.getConfig()
		start := time.Now()
		online := doPing(cfg)
		interval := time.Duration(cfg.PingIntervalOfflineSec) * time.Second
		if online {
			interval = time.Duration(cfg.PingIntervalOnlineSec) * time.Second
		}
		wait := interval - time.Since(start)
		if wait < 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		timer.Reset(wait)
	}
}

func makeFont(name string, height int32, weight int32) uintptr {
	u16, _ := syscall.UTF16PtrFromString(name)
	h, _, _ := procCreateFontW.Call(
		uintptr(uint32(height)),
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, 0,
		ANTIALIASED_QUALITY,
		0,
		uintptr(unsafe.Pointer(u16)),
	)
	return h
}

func hideOwnConsole() {
	chwnd, _, _ := procGetConsoleWindow.Call()
	if chwnd == 0 {
		return
	}
	var pids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), 2)
	// Only hide/free if this process is the sole owner of the console window
	if n == 1 {
		procShowWindow.Call(chwnd, 0) // SW_HIDE
		procFreeConsole.Call()
	}
}

func main() {
	runtime.LockOSThread()
	hideOwnConsole()

	// Enable DPI awareness
	if r, _, _ := procSetProcessDpiAwareness.Call(1); r != 0 {
		procSetProcessDPIAware.Call()
	}

	envFile := getEnvFilePath()
	cfg, existed := loadConfig(envFile)

	app = &App{
		cfg:         cfg,
		envPath:     envFile,
		alwaysOnTop: cfg.AlwaysOnTop,
		status:      stateChecking,
		fontTitle:   makeFont("Segoe UI", -15, FW_SEMIBOLD),
		fontSub:     makeFont("Segoe UI", -12, FW_NORMAL),
		pingNowCh:   make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
	}
	defer procDeleteObject.Call(app.fontTitle)
	defer procDeleteObject.Call(app.fontSub)

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("NetCheckGoWidgetClass")
	winTitle, _ := syscall.UTF16PtrFromString("NetCheck")
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)

	wc := WNDCLASSEXW{
		Size:      uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  hInst,
		Cursor:    cursor,
		ClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	exStyle := uintptr(WS_EX_LAYERED | WS_EX_TOOLWINDOW)
	if app.alwaysOnTop {
		exStyle |= WS_EX_TOPMOST
	}
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		WS_POPUP|WS_VISIBLE,
		0, 0, winW, winH,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return
	}
	app.hwnd = hwnd

	if cfg.HasPos && isPosVisible(cfg.PosX, cfg.PosY) {
		app.moveTo(cfg.PosX, cfg.PosY)
		if !existed {
			app.persistCurrentState()
		}
	} else {
		app.snapToCorner("br")
	}
	app.render()
	procShowWindow.Call(hwnd, SW_SHOW)

	go app.pingLoop()

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
