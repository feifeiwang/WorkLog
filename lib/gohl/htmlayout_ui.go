package gohl

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

func init() {
	//take care!  
	//主goroutine必须锁定在主线程，否则被调试之后，导致HTMLayout崩溃（GUI操作需要在主线程）
	runtime.LockOSThread()
}

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	user32                         = syscall.NewLazyDLL("user32.dll")
	dwmapi                         = syscall.NewLazyDLL("dwmapi.dll")
	gdi32                          = syscall.NewLazyDLL("gdi32.dll")
	procLoadIcon                   = user32.NewProc("LoadIconW")
	procIsZoomed                   = user32.NewProc("IsZoomed")
	procGetWindowRect              = user32.NewProc("GetWindowRect")
	procCreateWindowEx             = user32.NewProc("CreateWindowExW")
	procDefWindowProc              = user32.NewProc("DefWindowProcW")
	procRegisterClassEx            = user32.NewProc("RegisterClassExW")
	procGetMessage                 = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessage            = user32.NewProc("DispatchMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procLoadCursor                 = user32.NewProc("LoadCursorW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procUpdateWindow               = user32.NewProc("UpdateWindow")
	procGetModuleHandle            = kernel32.NewProc("GetModuleHandleW")
	procGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procSetWindowText              = user32.NewProc("SetWindowTextW")
	procScreenToClient             = user32.NewProc("ScreenToClient")
	procDwmSetWindowAttr           = dwmapi.NewProc("DwmSetWindowAttribute")
	procSetTimer                   = user32.NewProc("SetTimer")
	procKillTimer                  = user32.NewProc("KillTimer")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procUpdateLayeredWindow        = user32.NewProc("UpdateLayeredWindow")
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                   = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procCreateRoundRectRgn         = gdi32.NewProc("CreateRoundRectRgn")
	procSetWindowRgn               = user32.NewProc("SetWindowRgn")
	procGetWindowLong              = user32.NewProc("GetWindowLongW")
	procSetWindowLong              = user32.NewProc("SetWindowLongW")
)

const (
	WS_OVERLAPPED       = 0x00000000
	WS_POPUP            = 0x80000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX
	WS_CLIPCHILDREN     = 0x02000000
	WS_CLIPSIBLINGS     = 0x04000000
	WS_EX_LAYERED       = 0x00080000

	WM_DESTROY    = 0x0002
	WM_CREATE     = 0x0001
	WM_CLOSE      = 0x0010
	WM_ERASEBKGND = 0x0014
	WM_NCHITTEST  = 0x0084
	WM_NCCALCSIZE = 0x0083
	WM_NCPAINT    = 0x0085
	WM_NCACTIVATE = 0x0086
	WM_TIMER      = 0x0113
	WM_GETMINMAXINFO = 0x0024

	HTCLIENT      = 1
	HTCAPTION     = 2
	HTLEFT        = 10
	HTRIGHT       = 11
	HTTOP         = 12
	HTTOPLEFT     = 13
	HTTOPRIGHT    = 14
	HTBOTTOM      = 15
	HTBOTTOMLEFT  = 16
	HTBOTTOMRIGHT = 17

	IDC_ARROW = 32512

	CS_HREDRAW    = 0x0002
	CS_VREDRAW    = 0x0001
	CS_DROPSHADOW = 0x00020000

	SW_SHOW       = 5
	ERROR_SUCCESS = 0
	CW_USEDEFAULT = 0x80000000

	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	SWP_NOZORDER = 0x0004

	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWA_SYSTEMBACKDROP_TYPE      = 38
	DWMWCP_ROUND                   = 2

	LWA_COLORKEY = 0x00000001
	LWA_ALPHA    = 0x00000002

	ULW_COLORKEY = 0x00000001
	ULW_ALPHA    = 0x00000002
	ULW_OPAQUE   = 0x00000004

	GWL_EXSTYLE = -20
)

type wndClassEx struct {
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

type Msg struct {
	Hwnd    uint32
	Message uint32
	Wparam  uintptr
	Lparam  uintptr
	Time    uint32
	Pt      Point
}

type WindowConfig struct {
	Title        string
	Width        int
	Height       int
	ClassName    string
	Border       bool
	Frameless    bool // 无边框模式
	MaxBtn       bool
	MinBtn       bool
	Resize       bool
	Center       bool
	Icon         uintptr // 窗口图标 (HICON)
	Rounded      bool    // 圆角窗口
	CornerRadius int     // 圆角半径，默认10
	MinWidth     int     // 最小跟踪宽度（0=不限），WM_GETMINMAXINFO 生效
	MinHeight    int     // 最小跟踪高度（0=不限）
}

// minMaxInfo 对应 Win32 MINMAXINFO（用于 WM_GETMINMAXINFO 限制拖动最小尺寸）
type minMaxInfo struct {
	PtReserved    [2]int32
	PtMaxSize     [2]int32
	PtMaxPosition [2]int32
	PtMinTrackSize [2]int32
	PtMaxTrackSize [2]int32
}

type U struct {
	ID     string      `json:"id"`
	Action string      `json:"action"`
	Value  interface{} `json:"value"`
}

type ElementHandler func(elem *Element)
type MouseHandler func(elem *Element, params *MouseParams) bool

type Window struct {
	hwnd          uint32
	config        WindowConfig
	notifyHandler *NotifyHandler
	eventHandler  *EventHandler
	htmlContent   string
	loadFile      string
	onCreate      func()
	timers        map[int]func()
	nextTimerId   int
	timerHandler  *EventHandler

	OnMouseDown              MouseHandler
	OnMouseUp                MouseHandler
	OnClick                  ElementHandler
	OnEditValueChanging      ElementHandler
	OnEditValueChanged       ElementHandler
	OnSelectSelectionChanged ElementHandler
	OnButtonStateChanged     ElementHandler
	OnMenuItemClick          ElementHandler
	OnHyperlinkClick         ElementHandler
}

func NewWindow(config WindowConfig) *Window {
	if config.ClassName == "" {
		config.ClassName = "HTMLayoutWindow"
	}
	if config.Width == 0 {
		config.Width = 400
	}
	if config.Height == 0 {
		config.Height = 300
	}
	return &Window{
		config: config,
	}
}

func (w *Window) SetHtml(html string) *Window {
	w.htmlContent = html
	return w
}

func (w *Window) LoadFile(uri string) *Window {
	w.htmlContent = ""
	absPath, err := filepath.Abs(uri)
	if err != nil {
		log.Println("LoadFile error:", err)
		w.loadFile = uri
	} else {
		w.loadFile = "file:///" + filepath.ToSlash(absPath)
	}
	log.Println("LoadFile:", w.loadFile)
	return w
}

func (w *Window) SetEventHandler(handler *EventHandler) *Window {
	w.eventHandler = handler
	return w
}

func (w *Window) SetNotifyHandler(handler *NotifyHandler) *Window {
	if handler.OnLoadData == nil {
		handler.OnLoadData = defaultOnLoadData
	}
	w.notifyHandler = handler
	return w
}

var loadedResources = make(map[string][]byte)

func defaultOnLoadData(params *NmhlLoadData) uintptr {
	if params.Uri == nil {
		return 0
	}

	uri := utf16ToString(params.Uri)
	if len(uri) < 11 || uri[:11] != "resources://" {
		return 0
	}

	resourceName := uri[11:]
	filePath := filepath.Join(resourcesDir, resourceName)

	data, err := readFileBytes(filePath)
	if err != nil {
		log.Printf("[HLN_LOAD_DATA] Failed to load resource: %s, error: %v", filePath, err)
		return 0
	}

	loadedResources[uri] = data

	params.OutData = uintptr(unsafe.Pointer(&data[0]))
	params.OutDataSize = int32(len(data))
	params.DataType = 0

	log.Printf("[HLN_LOAD_DATA] Loaded resource: %s (%d bytes)", filePath, len(data))
	return 1
}

func readFileBytes(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	return data, err
}

func (w *Window) OnCreateCallback(fn func()) *Window {
	w.onCreate = fn
	return w
}

func (w *Window) GetHwnd() uint32 {
	return w.hwnd
}

func (w *Window) GetRootElement() *Element {
	return RootElement(w.hwnd)
}

func (w *Window) GetElementById(id string) *Element {
	root := RootElement(w.hwnd)
	if root == nil {
		return nil
	}
	return root.GetElementById(id)
}

func (w *Window) Run() {
	className := syscall.StringToUTF16Ptr(w.config.ClassName)
	hInstance, _, _ := procGetModuleHandle.Call()
	cursor, _, _ := procLoadCursor.Call(IDC_ARROW)

	classStyle := uint32(CS_HREDRAW | CS_VREDRAW)
	if w.config.Frameless {
		classStyle |= CS_DROPSHADOW
	}

	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		Style:      classStyle,
		WndProc:    syscall.NewCallback(w.wndProc),
		ClsExtra:   0,
		WndExtra:   0,
		Instance:   hInstance,
		Icon:       w.config.Icon,
		Cursor:     cursor,
		Background: 6,
		MenuName:   nil,
		ClassName:  className,
		IconSm:     w.config.Icon,
	}

	if _, errno := registerClassEx(&wc); errno != ERROR_SUCCESS {
		log.Panic("Failed to register window class: ", errno)
	}

	// 窗口样式设置
	var style uint32
	if w.config.Frameless {
		// 无边框模式
		style = WS_POPUP | WS_CLIPCHILDREN | WS_CLIPSIBLINGS
		if w.config.Resize {
			style |= WS_THICKFRAME
		}
	} else if w.config.Border {
		// 自定义边框 - 有边框但无标题栏
		style = WS_POPUP | WS_THICKFRAME | WS_CLIPCHILDREN | WS_CLIPSIBLINGS
	} else {
		// 标准窗口
		style = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_CLIPCHILDREN | WS_CLIPSIBLINGS
		if w.config.MaxBtn {
			style |= WS_MAXIMIZEBOX
		}
		if w.config.MinBtn {
			style |= WS_MINIMIZEBOX
		}
		if w.config.Resize {
			style |= WS_THICKFRAME
		}
	}

	x := uintptr(CW_USEDEFAULT)
	y := uintptr(CW_USEDEFAULT)
	width := uintptr(w.config.Width)
	height := uintptr(w.config.Height)

	if w.config.Center {
		screenWidth, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
		screenHeight, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
		x = uintptr(int(screenWidth)-w.config.Width) / 2
		y = uintptr(int(screenHeight)-w.config.Height) / 2
	}

	hwnd, errno := createWindowEx(
		0, className,
		syscall.StringToUTF16Ptr(w.config.Title),
		uint32(style),
		x, y,
		width, height,
		0, 0, hInstance, 0)
	if errno != ERROR_SUCCESS {
		log.Panic("Failed to create window: ", errno)
	}

	w.hwnd = hwnd

	if w.config.Rounded && w.config.Frameless {
		radius := w.config.CornerRadius
		if radius <= 0 {
			radius = 10
		}
		setRoundedRegion(uint32(hwnd), w.config.Width, w.config.Height, radius)
	}

	procShowWindow.Call(uintptr(hwnd), SW_SHOW)
	procUpdateWindow.Call(uintptr(hwnd))

	var msg Msg
	for {
		if r, errno := getMessage(&msg, 0, 0, 0); errno != ERROR_SUCCESS {
			log.Panic("GetMessage error: ", errno)
		} else if r == 0 {
			break
		}
		translateMessage(&msg)
		dispatchMessage(&msg)
	}
}

func (w *Window) wndProc(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	result, handled := ProcNoDefault(uint32(hwnd), msg, wparam, lparam)
	if handled {
		return result
	}

	switch msg {
	case WM_CREATE:
		w.hwnd = uint32(hwnd)
		// HTMLAYOUT_FONT_SMOOTHING = 4, // value: 0 - system default, 1 - no smoothing, 2 - std smoothing, 3 - clear type
		SetOption(uint32(hwnd), HTMLAYOUT_FONT_SMOOTHING, 4)
		SetOption(uint32(hwnd), HTMLAYOUT_ANIMATION_THREAD, 1)
		if w.notifyHandler != nil {
			AttachNotifyHandler(uint32(hwnd), w.notifyHandler)
		}
		// 始终设置默认事件处理器（处理窗口控制属性）
		if w.eventHandler == nil {
			w.setupDefaultEventHandler()
		}
		AttachWindowEventHandler(uint32(hwnd), w.eventHandler)
		if w.htmlContent != "" {
			err := LoadHtml(uint32(hwnd), []byte(w.htmlContent), "")
			if err != nil {
				log.Println("LoadHtml error:", err)
			}
		} else if w.loadFile != "" {
			err := LoadResource(uint32(hwnd), w.loadFile)
			if err != nil {
				log.Println("LoadResource error:", err)
			}
		}
		if w.onCreate != nil {
			w.onCreate()
		}
		return 0

	case WM_ERASEBKGND:
		return 1

	case WM_NCHITTEST:
		if w.config.Frameless {
			x := int(int16(lparam & 0xFFFF))
			y := int(int16((lparam >> 16) & 0xFFFF))
			ht := w.hitTest(x, y)
			if ht != HTCLIENT {
				return uintptr(ht)
			}
		}

	case WM_NCCALCSIZE:
		if w.config.Frameless {
			return 0
		}

	case WM_GETMINMAXINFO:
		// 让 Windows 自己夹住拖动缩放的最小尺寸（比定时器 SetWindowPos 抢回更平滑，
		// 不会与拖动模态循环打架造成抖动）
		if w.config.MinWidth > 0 || w.config.MinHeight > 0 {
			mmi := (*minMaxInfo)(unsafe.Pointer(lparam))
			if w.config.MinWidth > 0 {
				mmi.PtMinTrackSize[0] = int32(w.config.MinWidth)
			}
			if w.config.MinHeight > 0 {
				mmi.PtMinTrackSize[1] = int32(w.config.MinHeight)
			}
			return 0
		}

	case WM_NCPAINT:
		if w.config.Frameless {
			return 0
		}

	case WM_NCACTIVATE:
		if w.config.Frameless {
			if wparam == 0 {
				return 1
			}
			return 0
		}

	case WM_CLOSE:
		log.Printf("[WM_CLOSE] hwnd=%d, w.hwnd=%d, eventHandler=%v", hwnd, w.hwnd, w.eventHandler != nil)
		if w.eventHandler != nil {
			DetachWindowEventHandler(w.hwnd)
			w.eventHandler = nil
		}
		if w.notifyHandler != nil {
			DetachNotifyHandler(w.hwnd)
			w.notifyHandler = nil
		}
		destroyWindow(w.hwnd)
		return 0

	case WM_DESTROY:
		postQuitMessage(0)
		return 0
	}

	return defWindowProc(uint32(hwnd), msg, wparam, lparam)
}

func (w *Window) hitTest(screenX, screenY int) int {
	// 将屏幕坐标转换为窗口客户区坐标
	pt := struct{ X, Y int32 }{int32(screenX), int32(screenY)}
	procScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&pt)))

	// 查找该位置的元素
	elem := FindElement(w.hwnd, int(pt.X), int(pt.Y))
	if elem == nil {
		return HTCLIENT
	}

	// 检查元素及其父元素是否有 -gohl-drag 属性；
	// 但若链上存在可交互元素（data-act 按钮、输入框等），则不作为拖动区，保证点击可用
	interactive := false
	draggable := false
	for e := elem; e != nil; e = e.Parent() {
		if !interactive {
			if _, hasAct := e.Attr("data-act"); hasAct {
				interactive = true
			}
			switch e.Type() {
			case "input", "textarea", "select":
				interactive = true
			}
		}
		if _, hasDrag := e.Attr("-gohl-drag"); hasDrag {
			draggable = true
		}
	}
	if draggable && !interactive {
		return HTCAPTION
	}

	// 检查是否在边框区域（用于调整窗口大小）
	if w.config.Resize {
		var rect struct{ Left, Top, Right, Bottom int32 }
		procGetWindowRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&rect)))

		borderWidth := int32(5)

		onLeft := pt.X < borderWidth
		onRight := pt.X > (rect.Right - rect.Left - borderWidth)
		onTop := pt.Y < borderWidth
		onBottom := pt.Y > (rect.Bottom - rect.Top - borderWidth)

		if onTop && onLeft {
			return HTTOPLEFT
		}
		if onTop && onRight {
			return HTTOPRIGHT
		}
		if onBottom && onLeft {
			return HTBOTTOMLEFT
		}
		if onBottom && onRight {
			return HTBOTTOMRIGHT
		}
		if onTop {
			return HTTOP
		}
		if onBottom {
			return HTBOTTOM
		}
		if onLeft {
			return HTLEFT
		}
		if onRight {
			return HTRIGHT
		}
	}

	return HTCLIENT
}

func (w *Window) setupDefaultEventHandler() {
	w.eventHandler = &EventHandler{
		OnMouse: func(he HELEMENT, params *MouseParams) bool {
			cmd := params.Cmd & 0xFF
			target := NewElementFromHandle(params.Target)

			switch cmd {
			case MOUSE_DOWN:
				if w.OnMouseDown != nil {
					return w.OnMouseDown(target, params)
				}
			case MOUSE_UP:
				if w.OnMouseUp != nil {
					return w.OnMouseUp(target, params)
				}
			}
			return false
		},
		OnBehaviorEvent: func(he HELEMENT, params *BehaviorEventParams) bool {
			elem := NewElementFromHandle(params.Target)

			switch params.Cmd {
			case BUTTON_CLICK:
				if _, hasMin := elem.Attr("-gohl-min"); hasMin {
					w.Minimize()
					return true
				}
				if _, hasMax := elem.Attr("-gohl-max"); hasMax {
					isMaximized, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
					if isMaximized != 0 {
						w.Restore()
					} else {
						w.Maximize()
					}
					return true
				}
				if _, hasClose := elem.Attr("-gohl-close"); hasClose {
					w.Close()
					return true
				}
				if w.OnClick != nil {
					w.OnClick(elem)
					return true
				}

			case EDIT_VALUE_CHANGING:
				if w.OnEditValueChanging != nil {
					w.OnEditValueChanging(elem)
					return false
				}

			case EDIT_VALUE_CHANGED:
				if w.OnEditValueChanged != nil {
					w.OnEditValueChanged(elem)
					return false
				}

			case SELECT_SELECTION_CHANGED:
				if w.OnSelectSelectionChanged != nil {
					w.OnSelectSelectionChanged(elem)
					return false
				}

			case BUTTON_STATE_CHANGED:
				if w.OnButtonStateChanged != nil {
					w.OnButtonStateChanged(elem)
					return false
				}

			case MENU_ITEM_CLICK:
				if w.OnMenuItemClick != nil {
					w.OnMenuItemClick(elem)
					return true
				}

			case HYPERLINK_CLICK:
				if w.OnHyperlinkClick != nil {
					w.OnHyperlinkClick(elem)
					return true
				}
			}
			return false
		},
		OnTimer: func(he HELEMENT, params *TimerParams) bool {
			timerId := int(params.TimerId)
			log.Printf("[OnTimer] timerId=%d", timerId)
			if w.timers != nil {
				if callback, exists := w.timers[timerId]; exists {
					callback()
					delete(w.timers, timerId)
					return true
				}
			}
			return false
		},
	}
}

func (w *Window) GetElementValue(id string) string {
	root := RootElement(w.hwnd)
	if root == nil {
		return ""
	}
	elements := root.Select("#" + id)
	if len(elements) == 0 {
		return ""
	}
	elem := elements[0]

	if val, err := elem.ValueAsString(); err == nil {
		return val
	}

	if val := elem.Text(); val != "" {
		return val
	}

	return ""
}

func (w *Window) SetElementValue(id string, value string) {
	root := RootElement(w.hwnd)
	if root == nil {
		return
	}
	elements := root.Select("#" + id)
	if len(elements) > 0 {
		elem := elements[0]
		elemType := elem.Type()
		if elemType == "select" || elemType == "input" || elemType == "textarea" {
			elem.SetValue(value)
		}
	}
}

func (w *Window) UpdateUI(updates ...U) {
	root := RootElement(w.hwnd)
	if root == nil {
		return
	}

	for _, update := range updates {
		elem := root.GetElementById(update.ID)
		if elem == nil {
			continue
		}

		switch update.Action {
		case "text":
			if val, ok := update.Value.(string); ok {
				elem.SetText(val)
			}
		case "html":
			if val, ok := update.Value.(string); ok {
				elem.SetHtml(val)
			}
		case "value":
			if val, ok := update.Value.(string); ok {
				elem.SetValue(val)
			}
		case "class":
			if val, ok := update.Value.(string); ok {
				elem.SetAttr("class", val)
			}
		case "addClass":
			if val, ok := update.Value.(string); ok {
				currentClass, _ := elem.Attr("class")
				if currentClass != "" {
					elem.SetAttr("class", currentClass+" "+val)
				} else {
					elem.SetAttr("class", val)
				}
			}
		case "removeClass":
			if val, ok := update.Value.(string); ok {
				currentClass, _ := elem.Attr("class")
				if currentClass != "" {
					classes := removeClass(currentClass, val)
					elem.SetAttr("class", classes)
				}
			}
		case "show":
			elem.Show()
		case "hide":
			elem.Hide()
		case "attr":
			if val, ok := update.Value.(map[string]interface{}); ok {
				for k, v := range val {
					if vs, ok := v.(string); ok {
						elem.SetAttr(k, vs)
					}
				}
			}
		case "style":
			if val, ok := update.Value.(map[string]interface{}); ok {
				for k, v := range val {
					if vs, ok := v.(string); ok {
						elem.SetStyle(k, vs)
					}
				}
			}
		case "enabled":
			if val, ok := update.Value.(bool); ok {
				if val {
					elem.SetState(STATE_DISABLED, false)
				} else {
					elem.SetState(STATE_DISABLED, true)
				}
			}
		}
	}
}

func (w *Window) SetTimer(milliseconds uint, callback func()) int {
	if w.timers == nil {
		w.timers = make(map[int]func())
	}
	w.nextTimerId++
	timerId := w.nextTimerId
	w.timers[timerId] = callback

	root := RootElement(w.hwnd)
	if root == nil {
		log.Println("[SetTimer] root is nil!")
		return 0
	}

	if w.timerHandler == nil {
		w.timerHandler = &EventHandler{
			OnTimer: func(he HELEMENT, params *TimerParams) bool {
				tid := int(params.TimerId)
				if cb, exists := w.timers[tid]; exists {
					delete(w.timers, tid)
					root := RootElement(w.hwnd)
					if root != nil {
						root.SetTimer(0, uintptr(tid))
					}
					cb()
				}
				return true
			},
		}
	}
	//必须在SetTimer之前调用AttachHandler,才可以正常触发事件
	root.AttachHandler(w.timerHandler)
	root.SetTimer(milliseconds, uintptr(timerId))
	log.Printf("[SetTimer] timerId=%d, ms=%d", timerId, milliseconds)
	return timerId
}

func (w *Window) KillTimer(timerId int) {
	root := RootElement(w.hwnd)
	if root != nil {
		root.SetTimer(0, uintptr(timerId))
	}
	delete(w.timers, timerId)
}

// 窗口控制方法
func (w *Window) Minimize() {
	procShowWindow.Call(uintptr(w.hwnd), 6) // SW_MINIMIZE
}

func (w *Window) Maximize() {
	procShowWindow.Call(uintptr(w.hwnd), 3) // SW_MAXIMIZE
}

func (w *Window) Restore() {
	procShowWindow.Call(uintptr(w.hwnd), 9) // SW_RESTORE
}

func (w *Window) Close() {
	procDestroyWindow.Call(uintptr(w.hwnd))
}

func (w *Window) SetTitle(title string) {
	procSetWindowText.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))))
}

func (w *Window) Show() {
	procShowWindow.Call(uintptr(w.hwnd), 5) // SW_SHOW
	procUpdateWindow.Call(uintptr(w.hwnd))
}

func LoadIconFromResource(id int) uintptr {
	// 获取当前模块句柄
	hInstance, _, _ := procGetModuleHandle.Call(0)
	// 加载图标资源
	// 加载 ID 为 1 的图标资源
	icon, _, _ := procLoadIcon.Call(hInstance, uintptr(id))
	return icon
}

func removeClass(classStr, removeClass string) string {
	classes := make([]string, 0)
	start := 0
	for i := 0; i <= len(classStr); i++ {
		if i == len(classStr) || classStr[i] == ' ' {
			if i > start {
				cls := classStr[start:i]
				if cls != removeClass {
					classes = append(classes, cls)
				}
			}
			start = i + 1
		}
	}
	result := ""
	for i, cls := range classes {
		if i > 0 {
			result += " "
		}
		result += cls
	}
	return result
}

func registerClassEx(wndclass *wndClassEx) (atom uint16, err syscall.Errno) {
	r0, _, e1 := syscall.SyscallN(procRegisterClassEx.Addr(), uintptr(unsafe.Pointer(wndclass)))
	atom = uint16(r0)
	if atom == 0 {
		if e1 != 0 {
			err = syscall.Errno(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func createWindowEx(exstyle uint32, classname *uint16, windowname *uint16, style uint32, x uintptr, y uintptr, width uintptr, height uintptr, wndparent uint32, menu uint32, instance uintptr, param uintptr) (hwnd uint32, err syscall.Errno) {
	r0, _, e1 := syscall.SyscallN(procCreateWindowEx.Addr(),
		uintptr(exstyle), uintptr(unsafe.Pointer(classname)), uintptr(unsafe.Pointer(windowname)),
		uintptr(style), x, y, width, height, uintptr(wndparent), uintptr(menu), instance, param)
	hwnd = uint32(r0)
	if hwnd == 0 {
		if e1 != 0 {
			err = syscall.Errno(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func defWindowProc(hwnd uint32, msg uint32, wparam uintptr, lparam uintptr) (lresult uintptr) {
	r0, _, _ := syscall.SyscallN(procDefWindowProc.Addr(), uintptr(hwnd), uintptr(msg), wparam, lparam)
	lresult = r0
	return
}

func getMessage(msg *Msg, hwnd uint32, MsgFilterMin uint32, MsgFilterMax uint32) (ret int32, err syscall.Errno) {
	r0, _, e1 := syscall.SyscallN(procGetMessage.Addr(), uintptr(unsafe.Pointer(msg)), uintptr(hwnd), uintptr(MsgFilterMin), uintptr(MsgFilterMax))
	ret = int32(r0)
	if ret == -1 {
		if e1 != 0 {
			err = syscall.Errno(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func translateMessage(msg *Msg) (done bool) {
	r0, _, _ := syscall.SyscallN(procTranslateMessage.Addr(), uintptr(unsafe.Pointer(msg)))
	done = bool(r0 != 0)
	return
}

func dispatchMessage(msg *Msg) (ret uintptr) {
	r0, _, _ := syscall.SyscallN(procDispatchMessage.Addr(), uintptr(unsafe.Pointer(msg)))
	ret = r0
	return
}

func destroyWindow(hwnd uint32) (err syscall.Errno) {
	r1, _, e1 := syscall.SyscallN(procDestroyWindow.Addr(), uintptr(hwnd))
	if int(r1) == 0 {
		if e1 != 0 {
			err = syscall.Errno(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func postQuitMessage(exitcode int32) {
	syscall.SyscallN(procPostQuitMessage.Addr(), uintptr(exitcode))
}

func setRoundedRegion(hwnd uint32, width, height, radius int) {
	var cornerPreference uint32 = DWMWCP_ROUND
	ret, _, _ := procDwmSetWindowAttr.Call(
		uintptr(hwnd),
		uintptr(DWMWA_WINDOW_CORNER_PREFERENCE),
		uintptr(unsafe.Pointer(&cornerPreference)),
		unsafe.Sizeof(cornerPreference),
	)
	if ret == 0 {
		return
	}

	r0, _, _ := procCreateRoundRectRgn.Call(
		uintptr(0), uintptr(0),
		uintptr(width+1), uintptr(height+1),
		uintptr(radius), uintptr(radius),
	)
	if r0 != 0 {
		procSetWindowRgn.Call(uintptr(hwnd), r0, 1)
	}
}
