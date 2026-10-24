package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// ---------- Win32 API 助手（纯 syscall，零依赖，Win7 SP1 全支持） ----------

var (
	user32sys   = syscall.NewLazyDLL("user32.dll")
	gdi32sys    = syscall.NewLazyDLL("gdi32.dll")
	shell32sys  = syscall.NewLazyDLL("shell32.dll")
	comdlg32sys = syscall.NewLazyDLL("comdlg32.dll")
	ole32sys    = syscall.NewLazyDLL("ole32.dll")
	kernel32sys = syscall.NewLazyDLL("kernel32.dll")

	procSetWindowPos       = user32sys.NewProc("SetWindowPos")
	procGetClientRect      = user32sys.NewProc("GetClientRect")
	procSetWindowRgn       = user32sys.NewProc("SetWindowRgn")
	procShowWindowW        = user32sys.NewProc("ShowWindow")
	procIsZoomed           = user32sys.NewProc("IsZoomed")
	procSetProcessDPIAware = user32sys.NewProc("SetProcessDPIAware")
	procCreateRoundRectRgn = gdi32sys.NewProc("CreateRoundRectRgn")
	procGetOpenFileName    = comdlg32sys.NewProc("GetOpenFileNameW")
	procSHBrowseForFolder  = shell32sys.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDL   = shell32sys.NewProc("SHGetPathFromIDListW")
	procShellExecute       = shell32sys.NewProc("ShellExecuteW")
	procMessageBox         = user32sys.NewProc("MessageBoxW")
	procCoTaskMemFree      = ole32sys.NewProc("CoTaskMemFree")
	// procGetCursorPos 在 tray.go 已声明（同包共用），这里只补 GetAsyncKeyState
	procGetAsyncKeyState   = user32sys.NewProc("GetAsyncKeyState")
)

const (
	hwndTopmost    = ^uintptr(0)    // (HWND)-1
	hwndNoTopmost  = ^uintptr(1)    // (HWND)-2
	swpNoMoveSize  = 0x0002 | 0x0001 // SWP_NOSIZE | SWP_NOMOVE
	swpNoMoveNoZ   = 0x0002 | 0x0004 // SWP_NOMOVE | SWP_NOZORDER
	swShowNormal   = 1
	mbYesNo        = 0x00000004
	mbIconQuestion = 0x00000020
	mbIconWarning  = 0x00000030
	idYes          = 6
	ofnPathMustExist = 0x00000800
	ofnFileMustExist = 0x00001000
	ofnExplorer      = 0x00080000 // Vista/Win7 资源管理器风格对话框
	bifReturnOnlyFsDirs = 0x00000001
	bifNewDialogStyle    = 0x00000050
)

// setProcessDPIAware 声明系统 DPI 感知，避免高分屏（125%/150%）下窗口被
// 虚拟化拉伸：布局尺寸与实际像素不一致会导致内容裁切、截图错位。
func setProcessDPIAware() {
	procSetProcessDPIAware.Call()
}

// getCursorPos 取屏幕光标坐标（子界面拖拉缩放用）
func getCursorPos() (int, int) {
	var p struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}

// isLButtonDown 左键是否按住（拖拉循环中判断松手）
func isLButtonDown() bool {
	v, _, _ := procGetAsyncKeyState.Call(0x01) // VK_LBUTTON
	return v&0x8000 != 0
}

// getClientSize 取客户区宽高
func getClientSize(hwnd uintptr) (int, int) {	var r struct{ Left, Top, Right, Bottom int32 }
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// applyRoundedRegion 按当前客户区尺寸重设圆角窗口区域（缩放后调用，否则窗口形状停留在初始大小）
func applyRoundedRegion(hwnd uintptr, radius int) {
	w, h := getClientSize(hwnd)
	if w <= 0 || h <= 0 {
		return
	}
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1),
		uintptr(radius*2), uintptr(radius*2))
	if rgn == 0 {
		return
	}
	// SetWindowRgn 成功后区域归系统所有，无需手动释放
	procSetWindowRgn.Call(hwnd, rgn, 1)
}

// setTopmost 切换窗口置顶
func setTopmost(hwnd uintptr, on bool) {	var after uintptr = hwndNoTopmost
	if on {
		after = hwndTopmost
	}
	procSetWindowPos.Call(hwnd, after, 0, 0, 0, 0, swpNoMoveSize)
}

// isZoomed 窗口是否处于最大化状态
func isZoomed(hwnd uintptr) bool {
	r, _, _ := procIsZoomed.Call(hwnd)
	return r != 0
}

// clearWindowRegion 清除窗口区域（最大化时用直角，圆角区域会切掉四角）
func clearWindowRegion(hwnd uintptr) {
	procSetWindowRgn.Call(hwnd, 0, 1)
}

// openPath 打开链接：文件夹直接打开；文件则打开它所在的文件夹
// （不用默认程序打开文件，避免误改内容）
func openPath(hwnd uintptr, path string) {
	p := strings.TrimSpace(path)
	if p == "" {
		return
	}
	// 文件夹直接打开；文件一律打开它所在的文件夹（不用默认程序打开文件，
	// 避免误改内容；也不再 explorer /select 高亮——用户要求就是打开所在文件夹）
	dir := p
	if info, err := os.Stat(p); err == nil {
		if !info.IsDir() {
			dir = filepath.Dir(p)
		}
	} else {
		dir = filepath.Dir(p) // 路径已不存在：退到父目录，至少能看到所在位置
	}
	verb := utf16p("open")
	target := utf16p(dir)
	procShellExecute.Call(hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, swShowNormal)
}

func shellOpen(hwnd uintptr, verbName, path string) {
	verb := utf16p(verbName)
	p := utf16p(path)
	procShellExecute.Call(hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, swShowNormal)
}

// confirmBox 弹出「确定/取消」确认框，点确定返回 true（删除等危险操作二次确认用）
func confirmBox(hwnd uintptr, title, text string) bool {
	t := utf16p(title)
	tx := utf16p(text)
	r, _, _ := procMessageBox.Call(hwnd, uintptr(unsafe.Pointer(tx)), uintptr(unsafe.Pointer(t)), mbYesNo|mbIconQuestion)
	return r == idYes
}

// utf16p 转成以 NUL 结尾的 UTF-16 字符串指针
func utf16p(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

// ---------- 选择文件（GetOpenFileNameW） ----------

type openFileNameW struct {
	lStructSize       uint32
	_                 uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       uintptr
	lpstrCustomFilter uintptr
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         uintptr
	nMaxFile          uint32
	_                 uint32
	lpstrFileTitle    uintptr
	nMaxFileTitle     uint32
	_                 uint32
	lpstrInitialDir   uintptr
	lpstrTitle        uintptr
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       uintptr
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    uintptr
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// pickFile 弹出"选择文件"对话框，返回路径；取消返回 ""
func pickFile(hwnd uintptr) string {
	const maxPath = 4096
	buf := make([]uint16, maxPath)
	ofn := openFileNameW{
		hwndOwner:  hwnd,
		lpstrFile:  uintptr(unsafe.Pointer(&buf[0])),
		nMaxFile:   maxPath,
		lpstrTitle: utf16ptr("选择要附加的文件"),
		flags:      ofnPathMustExist | ofnFileMustExist | ofnExplorer,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// ---------- 选择文件夹（SHBrowseForFolderW） ----------

type browseInfoW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      uintptr
	ulFlags        uint32
	_              uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
	_              int32
}

// pickFolder 弹出"选择文件夹"对话框，返回路径；取消返回 ""
func pickFolder(hwnd uintptr) string {
	var display [260]uint16
	bi := browseInfoW{
		hwndOwner:      hwnd,
		pszDisplayName: &display[0],
		lpszTitle:      utf16ptr("选择要附加的文件夹"),
		ulFlags:        bifReturnOnlyFsDirs | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl) // 释放 PIDL，防泄漏

	var path [260]uint16
	r, _, _ := procSHGetPathFromIDL.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(path[:])
}

func utf16ptr(s string) uintptr {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	return uintptr(unsafe.Pointer(p))
}
