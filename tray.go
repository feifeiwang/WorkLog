package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"gohl"
)

//go:embed tray.ico
var trayIco []byte

// ---------- 系统托盘（Shell_NotifyIconW，Win7 SP1 全支持，纯 syscall 零依赖） ----------
//
// 实现要点：
//  1. 托盘需要一个能接收回调消息的窗口。这里创建一个隐藏的消息窗口，
//     与主窗口同属一个线程 —— gohl 的 Run() 消息循环会统一派发，
//     因此托盘事件全部在主线程执行，操作 UI 无需跨线程同步。
//  2. 最小化 = 隐藏主窗口（本程序是无边框 POPUP 窗口，本就不在任务栏显示），
//     托盘图标常驻，双击/菜单均可恢复。
//  3. 右键菜单：打开主界面 / 快速添加记事 / 快速添加待办 / 退出。

var (
	procShellNotifyIcon       = shell32sys.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu       = user32sys.NewProc("CreatePopupMenu")
	procAppendMenu            = user32sys.NewProc("AppendMenuW")
	procTrackPopupMenu        = user32sys.NewProc("TrackPopupMenu")
	procDestroyMenu           = user32sys.NewProc("DestroyMenu")
	procGetCursorPos          = user32sys.NewProc("GetCursorPos")
	procLoadImageW            = user32sys.NewProc("LoadImageW")
	procSetForegroundWindow   = user32sys.NewProc("SetForegroundWindow")
	procGetForegroundWindow   = user32sys.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID = user32sys.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput     = user32sys.NewProc("AttachThreadInput")
	procRegisterClassEx       = user32sys.NewProc("RegisterClassExW")
	procCreateWindowEx        = user32sys.NewProc("CreateWindowExW")
	procDefWindowProc         = user32sys.NewProc("DefWindowProcW")
	procGetModuleHandle       = kernel32sys.NewProc("GetModuleHandleW")
	procGetCurrentThreadID    = kernel32sys.NewProc("GetCurrentThreadId")

	// 托盘回调窗口过程（NewCallback 只能创建一次，重复调用会 panic）
	trayWndProcCB = syscall.NewCallback(trayWndProc)
)

const (
	wmTrayCallback = 0x8001 // WM_APP + 1，避免与系统 WM_USER 消息冲突

	nimAdd    = 0x0
	nimDelete = 0x2
	nifMsg    = 0x1
	nifIcon   = 0x2
	nifTip    = 0x4

	mfString    = 0x0
	mfSeparator = 0x800

	tpmReturnCmd  = 0x0100
	tpmRightClick = 0x0002

	imageIcon      = 1
	lrLoadFromFile = 0x0010

	swHide = 0

	wsPopup = 0x80000000

	// 托盘菜单项 ID
	trayMenuOpen = 1
	trayMenuNote = 2
	trayMenuTodo = 3
	trayMenuExit = 4

	// 托盘图标回调的鼠标消息（lparam）
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
)

// notifyIconDataW NOTIFYICONDATAW（x64 布局，Win7 shell32 使用）
type notifyIconDataW struct {
	cbSize           uint32
	_                uint32 // hWnd 8 字节对齐的填充
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateFlags     uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

// trayWndClassEx WNDCLASSEXW（与 gohl 内部定义同布局）
type trayWndClassEx struct {
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

var (
	trayHwnd    uintptr
	trayIconHnd uintptr

	// mainThreadID 主线程（窗口线程）ID，AttachThreadInput 需要。
	// 在 main() 里通过 GetCurrentThreadId 捕获（gohl 已 LockOSThread，main 与消息循环同线程）。
	mainThreadID uintptr
)

// trayLog 临时诊断：写 %APPDATA%\gohl\tray_debug.log
func trayLog(format string, args ...interface{}) {
	f, err := os.OpenFile(filepath.Join(gohl.GetResourcesDir(), "tray_debug.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}

// trayWndProc 隐藏消息窗口的过程：只关心托盘回调消息，其余交 DefWindowProc
func trayWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmTrayCallback:
		switch uint32(lparam) {
		case wmLButtonDblClk:
			restoreMainWindow()
		case wmRButtonUp:
			showTrayMenu(hwnd)
		}
		return 0
	}
	r, _, _ := syscall.SyscallN(procDefWindowProc.Addr(), hwnd, uintptr(msg), wparam, lparam)
	return r
}

// initTray 释放图标、创建隐藏消息窗口、添加托盘图标。必须在主线程、Run() 之前调用。
func initTray() {
	// 图标释放到 %APPDATA%\gohl\（与 htmlayout.dll / 字体同一目录），LoadImageW 从文件加载
	icoPath := filepath.Join(gohl.GetResourcesDir(), "app.ico")
	if err := os.WriteFile(icoPath, trayIco, 0644); err != nil {
		trayLog("write ico fail: %v", err)
		fmt.Println("写入托盘图标失败:", err)
		return
	}
	trayIconHnd, _, _ = syscall.SyscallN(procLoadImageW.Addr(),
		0, utf16ptr(icoPath), imageIcon, 0, 0, lrLoadFromFile)
	if trayIconHnd == 0 {
		trayLog("load ico fail: %s", icoPath)
		fmt.Println("加载托盘图标失败:", icoPath)
		return
	}
	trayLog("ico loaded ok")

	hInstance, _, _ := syscall.SyscallN(procGetModuleHandle.Addr(), 0)
	clsName := syscall.StringToUTF16Ptr("WorkLogTrayMsg")
	wc := trayWndClassEx{
		Size:      uint32(unsafe.Sizeof(trayWndClassEx{})),
		WndProc:   trayWndProcCB,
		Instance:  hInstance,
		ClassName: clsName,
	}
	atom, _, err := syscall.SyscallN(procRegisterClassEx.Addr(), uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		trayLog("register class fail: %v", err)
		fmt.Println("注册托盘窗口类失败:", err)
		return
	}

	// 不调用 ShowWindow：窗口存在但不可见，仍可接收消息。
	// 注意不能用 :=（会把包级 trayHwnd 遮蔽成局部变量，导致 addTrayIcon 拿到 0）
	hwndPtr, _, err2 := syscall.SyscallN(procCreateWindowEx.Addr(),
		0, uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("worklog-tray"))),
		uintptr(wsPopup), 0, 0, 0, 0, 0, 0, hInstance, 0)
	trayHwnd = hwndPtr
	if trayHwnd == 0 {
		trayLog("create msg window fail: %v", err2)
		fmt.Println("创建托盘消息窗口失败:", err2)
		return
	}
	trayLog("msg window ok hwnd=%d", trayHwnd)
	addTrayIcon()
}

func addTrayIcon() {
	if trayHwnd == 0 || trayIconHnd == 0 {
		trayLog("addIcon skip hwnd=%d icon=%d", trayHwnd, trayIconHnd)
		return
	}
	nid := notifyIconDataW{}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = trayHwnd
	nid.uID = 1
	nid.uFlags = nifMsg | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayCallback
	nid.hIcon = trayIconHnd
	copy(nid.szTip[:], syscall.StringToUTF16("工作记录"))
	ret, _, err2 := syscall.SyscallN(procShellNotifyIcon.Addr(), nimAdd, uintptr(unsafe.Pointer(&nid)))
	trayLog("Shell_NotifyIcon ret=%d err=%v structSize=%d", ret, err2, nid.cbSize)
}

func removeTrayIcon() {
	if trayHwnd == 0 {
		return
	}
	nid := notifyIconDataW{}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = trayHwnd
	nid.uID = 1
	syscall.SyscallN(procShellNotifyIcon.Addr(), nimDelete, uintptr(unsafe.Pointer(&nid)))
}

// hideToTray 最小化：隐藏主窗口，程序驻留托盘
func hideToTray() {
	procShowWindowW.Call(uintptr(app.gw.GetHwnd()), swHide)
}

// restoreMainWindow 从托盘恢复主窗口并激活
func restoreMainWindow() {
	hwnd := uintptr(app.gw.GetHwnd())
	app.gw.Show() // SW_SHOW + UpdateWindow，隐藏窗口恢复到此前的尺寸位置
	forceForegroundWindow(hwnd, 1500)
}

// exitApp 退出：先摘托盘图标再销毁窗口（否则进程退出前图标会残留成"幽灵图标"）
func exitApp() {
	removeTrayIcon()
	app.gw.Close()
}

// showTrayMenu 在光标处弹出托盘右键菜单（TPM_RETURNCMD 直接返回菜单 ID，无需处理 WM_COMMAND）
func showTrayMenu(hwnd uintptr) {
	menu, _, _ := syscall.SyscallN(procCreatePopupMenu.Addr())
	if menu == 0 {
		return
	}
	appendMenuItem(menu, mfString, trayMenuOpen, "打开主界面")
	appendMenuItem(menu, mfString, trayMenuNote, "快速添加记事")
	appendMenuItem(menu, mfString, trayMenuTodo, "快速添加待办")
	syscall.SyscallN(procAppendMenu.Addr(), menu, mfSeparator, 0, 0)
	appendMenuItem(menu, mfString, trayMenuExit, "退出")

	var pt struct{ X, Y int32 }
	syscall.SyscallN(procGetCursorPos.Addr(), uintptr(unsafe.Pointer(&pt)))

	// TrackPopupMenu 要求所属窗口在前台，否则点别处菜单不消失
	forceForegroundWindow(hwnd, 500)
	cmd, _, _ := syscall.SyscallN(procTrackPopupMenu.Addr(),
		menu, tpmRightClick|tpmReturnCmd,
		uintptr(uint32(pt.X)), uintptr(uint32(pt.Y)), // int32 负坐标防符号扩展
		0, hwnd, 0)
	syscall.SyscallN(procDestroyMenu.Addr(), menu)

	switch cmd {
	case trayMenuOpen:
		restoreMainWindow()
	case trayMenuNote:
		restoreMainWindow()
		hideAllPanels()
		openNoteDialog("")
	case trayMenuTodo:
		restoreMainWindow()
		hideAllPanels()
		openTodoDialog("")
	case trayMenuExit:
		exitApp()
	}
}

func appendMenuItem(menu uintptr, flags uintptr, id uintptr, text string) {
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	syscall.SyscallN(procAppendMenu.Addr(), menu, flags, id, uintptr(unsafe.Pointer(p)))
}

// forceForegroundWindow 尽力把窗口切到前台。Windows 限制非前台进程设置前台窗口，
// 常规做法是 AttachThreadInput 临时共享输入队列绕过。但该调用在前台线程无响应时
// （如锁屏的 winlogon）可能长时间阻塞，因此放在 goroutine 里执行并加等待超时，
// 保证主消息循环任何情况下都不会被卡住。
func forceForegroundWindow(hwnd uintptr, waitMs int) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fore, _, _ := syscall.SyscallN(procGetForegroundWindow.Addr())
		if fore == hwnd {
			return
		}
		foreThread, _, _ := syscall.SyscallN(procGetWindowThreadProcID.Addr(), fore, 0)
		if foreThread != 0 && foreThread != mainThreadID {
			syscall.SyscallN(procAttachThreadInput.Addr(), mainThreadID, foreThread, 1)
			syscall.SyscallN(procSetForegroundWindow.Addr(), hwnd)
			syscall.SyscallN(procAttachThreadInput.Addr(), mainThreadID, foreThread, 0)
			return
		}
		syscall.SyscallN(procSetForegroundWindow.Addr(), hwnd)
	}()
	select {
	case <-done:
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
		// 超时放弃（goroutine 可能仍阻塞在系统调用里，但不影响主循环）
	}
}
