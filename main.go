package main

import (
	_ "embed"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"gohl"
)

//go:embed ui.html
var uiHTML string

// ---------- 全局状态 ----------

type appState struct {
	data      *StoreData
	items     map[string][]Item // 项目 ID（文件基名）→ 该项目全部记录（"" = 未指定）
	probs     []Problem         // 问题排查（独立存储 问题排查/）
	gw       *gohl.Window
	filter   FilterState          // 关键词 + 日期区间
	recentDays int                // 记录流天数窗口（默认 defaultRecentDays，"加载更早" +5）
	editing  string               // 正在编辑的条目 ID（"" = 新增）
	draftLinks []string // 记事对话框暂存链接
	todoDraftLinks []string // 待办对话框暂存链接
	noteCatID     string   // 记事对话框当前选中的分类 ID（"" = 全部分类）
	todoCatID     string   // 待办对话框当前选中的分类 ID
	probCatID     string   // 问题对话框当前选中的分类 ID
	noteProjIDs []string  // 记事对话框选中的项目（可多选）
	todoProjIDs []string  // 待办对话框选中的项目（可多选）
	probProjIDs []string  // 问题对话框选中的项目（可多选）
	filterProjKW string   // 顶部项目栏的项目名搜索关键词
	draftSubs []SubTodo // 待办对话框暂存子待办

	// 问题排查对话框
	editingProb   string   // 正在编辑的问题 ID（"" = 新增）
	probDraftLinks []string // 问题对话框（档案）暂存链接
	probExpanded  map[string]bool // 展开中的问题卡片（默认折叠）
	lowOpen       bool     // 非紧急待办分组是否展开（默认 false = 收纳）

	// 纪实条目对话框
	logProbID   string // 目标问题 ID
	logEntryID  string // 正在编辑的纪实条目 ID（"" = 新增）
	logDraftLinks []string

	armedID  string    // 两步确认删除中的条目
	armedEl  *gohl.Element
	armedTmr int
	pendingRightID string // 右键按下待弹菜单的卡片 ID

	// 程序化回填输入框时抑制 OnEditValueChanged 误触发自动弹候选面板
	suppressEdit bool

	// HTMLayout date/time 输入的 value 属性在 change 事件当下才可靠，
	// 保存时读会拿到陈旧初值。故在 EDIT_VALUE_CHANGED 当下捕获到此 map，读取优先取。
	dateVals map[string]string

	// 子界面拖拉缩放（右下角把手）
	dlgDrag *dlgDragState
	// 项目管理界面正在编辑别名的项目 ID（"" = 无）
	editingAliasPID string
}

// dlgDragState 子界面拖拉缩放中的状态
type dlgDragState struct {
	kind          string // note/todo/proj/prob/log
	startX, startY int   // 按下时光标屏幕坐标
	startW, startH int   // 按下时对话框内容区尺寸
}

// loadMoreStepDays 点一次「加载更早」多显示的自然日数
const loadMoreStepDays = 5

var app *appState

func main() {
	setProcessDPIAware() // 高分屏下保证布局像素与物理像素一致
	// 捕获主线程 ID（gohl init 已 LockOSThread，main 与窗口消息循环同线程）
	mainThreadID, _, _ = syscall.SyscallN(procGetCurrentThreadID.Addr())

	registerEmbeddedFont()

	d, err := loadData()
	if err != nil {
		fmt.Println("读取数据失败，使用空数据:", err)
		d = &StoreData{Version: 1, FilterProjectID: FilterAll, Categories: []Category{}}
	}

	app = &appState{data: d, items: map[string][]Item{}, recentDays: defaultRecentDays, probExpanded: map[string]bool{}, dateVals: map[string]string{}}
	// 分类与项目 = 同目录下的「分类-项目.json」文件；逐个读入记录
	for _, pid := range d.allProjectIDs() {
		app.items[pid] = loadProjectItems(pid)
	}
	// 问题排查 = 问题排查/分类-项目-问题名称.json
	app.probs = scanProblems(d.allProjectIDs())
	save() // 首次运行即落盘，让用户立刻看到数据文件位置

	w := gohl.NewWindow(gohl.WindowConfig{
		Title:        "工作记录",
		Width:        616,
		Height:       800,
		ClassName:    "WorkLogWnd",
		Frameless:    true,
		Resize:       true, // 可拖动边缘缩放，像资源管理器窗口
		Rounded:      true,
		CornerRadius: 12,
		Center:       true,
		MinWidth:     minWinW, // WM_GETMINMAXINFO：Windows 原生夹住拖动最小尺寸
		MinHeight:    minWinH,
	})

	app.gw = w

	w.OnCreateCallback(func() {
		setTopmost(uintptr(w.GetHwnd()), d.Topmost)
		refreshTopmostBtn()
		renderFilterPanel()
		renderList()
		// 窗口缩放后同步圆角区域 / 最小尺寸（gohl 只在创建时设置一次圆角）
		app.watchResize()
		// 开发自测钩子：WORKLOG_UI_TEST=filter|date|proj|note|todo|noteproj 启动即打开对应界面
		uiTestHook()
		debugLayout("startup")
	})

	w.OnMouseDown = func(elem *gohl.Element, params *gohl.MouseParams) bool {
		return onMouseDown(elem, params)
	}
	w.OnMouseUp = func(elem *gohl.Element, params *gohl.MouseParams) bool {
		return onMouseUp(elem, params)
	}

	w.OnClick = func(elem *gohl.Element) {
		onClick(elem)
	}

	w.OnButtonStateChanged = func(elem *gohl.Element) {
		onCheckChanged(elem)
	}

	w.OnEditValueChanged = func(elem *gohl.Element) {
		if app.suppressEdit {
			return
		}
		id, ok := elem.Attr("id")
		if !ok {
			return
		}
		// date/time 输入：在 change 事件当下捕获 value（此后 Attr 可能回退陈旧值）
		if tv, _ := elem.Attr("type"); tv == "date" || tv == "time" {
			if v, ok2 := elem.Attr("value"); ok2 && app.dateVals != nil {
				app.dateVals[id] = v
			}
		}
		switch id {
		case "search-input":
			app.filter.Keyword = strings.TrimSpace(app.gw.GetElementValue("search-input"))
			app.recentDays = defaultRecentDays
			renderList()
		case "date-from", "date-to":
			// date 行为不支持 GET_TEXT_VALUE，必须走 Attr("value") 读取
			app.filter.DateFrom = sanitizeDate(inputValue("date-from"))
			app.filter.DateTo = sanitizeDate(inputValue("date-to"))
			app.recentDays = defaultRecentDays
			renderList()
		case "filter-search":
			// 项目栏搜索：只重绘列表部分，输入框本身不动，焦点不丢
			app.filterProjKW = strings.TrimSpace(app.gw.GetElementValue("filter-search"))
			renderFilterPanel()
	case "note-proj-input", "todo-proj-input", "prob-proj-input":
		// 项目号：仅当用户真正输入内容时才弹出候选面板（无需再点 ▼）；清空则收起
		kind := "note"
		if id == "todo-proj-input" {
			kind = "todo"
		} else if id == "prob-proj-input" {
			kind = "prob"
		}
		onProjInputChanged(kind)
		}
	}

	w.OnHyperlinkClick = func(elem *gohl.Element) {
		if href, ok := elem.Attr("href"); ok && href != "" {
			openPath(uintptr(w.GetHwnd()), href)
		}
	}

	// 托盘图标 + 隐藏消息窗口（须在主线程、Run 之前创建）
	initTray()

	// 乱码双保险：
	// 1) HTML 前加 UTF-8 BOM —— HTMLayout 无 BOM 时会按系统 ANSI 代码页（中文系统 GBK）解码
	// 2) 正文中文转 HTML 实体（纯 ASCII）—— 即使 BOM 被忽略也渲染正确
	w.SetHtml("\ufeff" + asciiSafeHTML(uiHTML)).Run()
}

// asciiSafeHTML 把 <style> 之外的非 ASCII 字符转为 HTML 实体，
// 避免 HTMLayout 按系统 ANSI 代码页解码 UTF-8 字节导致中文变问号。
func asciiSafeHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	inStyle := false
	for i := 0; i < len(s); {
		switch {
		case !inStyle && strings.HasPrefix(s[i:], "<style"):
			inStyle = true
			b.WriteString("<style")
			i += len("<style")
			continue
		case inStyle && strings.HasPrefix(s[i:], "</style>"):
			inStyle = false
			b.WriteString("</style>")
			i += len("</style>")
			continue
		}
		if inStyle {
			b.WriteByte(s[i])
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r < utf8.RuneSelf {
			b.WriteByte(byte(r))
		} else {
			// 用十进制实体：部分 HTML4 解析器不认十六进制 &#xNNNN;
			b.WriteString("&#")
			b.WriteString(strconv.Itoa(int(r)))
			b.WriteByte(';')
		}
		i += size
	}
	return b.String()
}

// ---------- 字体 ----------

var (
	procAddFontResource = syscall.NewLazyDLL("gdi32.dll").NewProc("AddFontResourceExW")
	fontRegistered      bool
)

func registerEmbeddedFont() {
	if fontRegistered {
		return
	}
	// 内嵌阿里巴巴普惠体已由 gohl 资源释放到 %APPDATA%/gohl，注册字体文件供 CSS 引用
	ttf := filepath.Join(gohl.GetResourcesDir(), "alibaba_puhui.ttf")
	p, err := syscall.UTF16PtrFromString(ttf)
	if err != nil {
		return
	}
	ret, _, _ := procAddFontResource.Call(uintptr(unsafe.Pointer(p)), 0x10, 0)
	if ret != 0 {
		fontRegistered = true
	}
}

// ---------- 保存 ----------

func save() {
	if err := saveData(app.data); err != nil {
		fmt.Println("保存失败:", err)
	}
}

// saveProject 把某个项目的记录写回其专属文件（分类-项目.json / 未指定.json）
func saveProject(pid string) {
	if err := saveProjectItems(pid, app.items[pid]); err != nil {
		fmt.Println("保存项目记录失败:", err)
	}
}

// rescanProjects 重新扫描 exe 同目录下的「分类-项目.json」文件：
// 文件消失 → 项目与记录一并移除（删除文件即删除项目）；新文件 → 读入。
// 问题排查目录同步重扫。
func rescanProjects() {
	app.data.Categories = scanCategories()
	for pid := range app.items {
		if pid == "" {
			continue
		}
		if _, _, ok := app.data.findProjectFull(pid); !ok {
			delete(app.items, pid)
		}
	}
	for _, pid := range app.data.allProjectIDs() {
		if _, ok := app.items[pid]; !ok {
			app.items[pid] = loadProjectItems(pid)
		}
	}
	app.probs = scanProblems(app.data.allProjectIDs())
	app.data.validateFilter()
	app.data.pruneAliasEntries() // 项目文件删除后清掉残留别名
}

// allRows 记事/待办 + 问题排查的统一列表
func (a *appState) allRows() []ListRow {
	var rows []ListRow
	for _, pid := range a.data.allProjectIDs() {
		for i := range a.items[pid] {
			it := a.items[pid][i]
			rows = append(rows, ListRow{Item: &it})
		}
	}
	for i := range a.probs {
		p := a.probs[i]
		rows = append(rows, ListRow{Prob: &p})
	}
	return rows
}

// findProb 按 ID 查问题（不存在返回 nil）
func (a *appState) findProb(id string) *Problem {
	for i := range a.probs {
		if a.probs[i].ID == id {
			return &a.probs[i]
		}
	}
	return nil
}

// allItems 汇总全部项目记录（含未指定）
func (a *appState) allItems() []Item {
	var out []Item
	for _, pid := range a.data.allProjectIDs() {
		out = append(out, a.items[pid]...)
	}
	return out
}

// findItemPos 按 ID 定位记录，返回（项目 ID, 下标）
func (a *appState) findItemPos(id string) (string, int) {
	for pid, its := range a.items {
		for i, it := range its {
			if it.ID == id {
				return pid, i
			}
		}
	}
	return "", -1
}

// ---------- 窗口缩放 ----------

// relayout 让 HTMLayout 按新的客户区尺寸重排（宽度/高度为弹性值时内容随之伸缩）
func relayout(hwnd uintptr) {
	if root := gohl.RootElement(uint32(hwnd)); root != nil {
		root.Update(true, true, true, true, true)
	}
}

// watchResize 定时检查窗口尺寸：拖拽缩放后重设圆角区域并重排内容
// 注意 gohl 的 SetTimer 是一次性的，回调末尾要重新注册
func (a *appState) watchResize() {
	hwnd := uintptr(a.gw.GetHwnd())
	lastW, lastH := 0, 0
	var tick func()
	tick = func() {
		w, h := getClientSize(hwnd)
		if w > 0 && h > 0 && (w != lastW || h != lastH) {
			first := lastW == 0
			lastW, lastH = w, h
			if isZoomed(hwnd) {
				clearWindowRegion(hwnd) // 最大化：直角
			} else {
				applyRoundedRegion(hwnd, 12)
			}
			if !first {
				relayout(hwnd) // 尺寸真变了才重排，避免启动时空重排
				recenterVisibleDialog()
				repositionVisiblePanels()
			}
			debugLayout(fmt.Sprintf("resize %dx%d", w, h))
		}
		// 最小尺寸保护：太窄对话框会溢出
		if w > 0 && (w < minWinW || h < minWinH) {
			nw, nh := w, h
			if nw < minWinW {
				nw = minWinW
			}
			if nh < minWinH {
				nh = minWinH
			}
			procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(nw), uintptr(nh), swpNoMoveNoZ)
		}
		a.gw.SetTimer(200, tick)
	}
	a.gw.SetTimer(200, tick)
}

// debugLayout 排查拉伸问题用：WORKLOG_DEBUG_LAYOUT=1 时把关键区块的实际
// 布局盒尺寸追加写到 %TEMP%\worklog_layout.log（不依赖截图，锁屏也能看）
func debugLayout(tag string) {
	if os.Getenv("WORKLOG_DEBUG_LAYOUT") == "" {
		return
	}
	log := func(format string, args ...interface{}) {
		f, err := os.OpenFile(filepath.Join(os.TempDir(), "worklog_layout.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, format+"\n", args...)
	}
	log("=== %s ===", tag)
	for _, id := range []string{"app", "topbar", "list-wrap", "list", "bottombar", "filter-panel", "date-panel"} {
		el := app.gw.GetElementById(id)
		if el == nil {
			log("  #%s = <nil>", id)
			continue
		}
		cw, ch := el.ContentBoxSize()
		log("  #%s box=%dx%d", id, cw, ch)
	}
	// 对话框：实际渲染位置（ContentViewBox = 视口坐标，可直接验证是否居中）
	dlgSaveBtn := map[string]string{
		"dlg-note": "note-save", "dlg-todo": "todo-save",
		"dlg-prob": "prob-save", "dlg-log": "log-save", "dlg-proj": "proj-close",
	}
	for _, id := range []string{"dlg-note", "dlg-todo", "dlg-prob", "dlg-log", "dlg-proj"} {
		if !panelVisible(app.gw.GetElementById(id + "-mask")) {
			continue
		}
		el := app.gw.GetElementById(id)
		if el == nil {
			continue
		}
		l, t, r, b := el.ContentViewBox()
		log("  #%s view=(%d,%d)-(%d,%d) size=%dx%d", id, l, t, r, b, r-l, b-t)
		// 内部溢出检查：保存/关闭按钮的底边不应超出对话框内容盒底边
		if act, ok := dlgSaveBtn[id]; ok {
			if btns := el.Select("[data-act=" + act + "]"); len(btns) > 0 {
				_, _, _, bb := btns[0].ContentViewBox()
				if bb > b+2 {
					log("  !! %s OVERFLOW: btn-bottom=%d > dlg-bottom=%d", id, bb, b)
				} else {
					log("  %s ok: btn-bottom=%d <= dlg-bottom=%d", id, bb, b)
				}
			}
		}
	}
	// 项目号下拉面板：可见性 + 选项数 + 提示条 + 已选 chips（验证下拉交互逻辑）
	for _, kind := range []string{"note", "todo", "prob"} {
		if !panelVisible(app.gw.GetElementById("dlg-" + kind + "-mask")) {
			continue
		}
		p := app.gw.GetElementById(kind + "-proj-panel")
		if p == nil {
			continue
		}
		head := ""
		if hs := p.Select(".dd-head"); len(hs) > 0 {
			head = strings.Join(strings.Fields(hs[0].Text()), " ")
		}
		chips := ""
		if sel := app.gw.GetElementById(kind + "-proj-sel"); sel != nil {
			chips = strings.Join(strings.Fields(sel.Text()), " ")
		}
		log("  #%s-proj-panel visible=%v items=%d head=%q sel=%q",
			kind, panelVisible(p), len(p.Select("[data-act=proj-pick]")), head, chips)
	}
	// 顶部面板：实际渲染位置（验证是否贴在按钮下方且不出界）
	for id := range panelAnchors {
		el := app.gw.GetElementById(id)
		if el == nil || !panelVisible(el) {
			continue
		}
		l, t, r, b := el.ContentViewBox()
		log("  #%s view=(%d,%d)-(%d,%d)", id, l, t, r, b)
	}
	// 列表结构：待办网格 / 天分组分割线 / 加载更早
	if list := app.gw.GetElementById("list"); list != nil {
		heads := list.Select(".zone-head")
		rows := list.Select(".todo-row")
		seps := list.Select(".day-sep-txt")
		more := list.Select(".load-more")
		log("  list: zone-head=%d todo-row=%d day-sep=%d load-more=%d",
			len(heads), len(rows), len(seps), len(more))
		for _, h := range heads {
			log("    zone-head = %s", h.Text())
		}
		for i, s := range seps {
			if i >= 8 {
				break
			}
			log("    day-sep[%d] = %s", i, s.Text())
		}
		for i, row := range rows {
			if n := len(row.Select(".todo-cell")); n != 2 {
				log("    !! todo-row[%d] cells=%d (want 2)", i, n)
			}
		}
		for _, b := range more {
			log("    load-more = %s", b.Text())
		}
	}
	if root := gohl.RootElement(app.gw.GetHwnd()); root != nil {
		rw, rh := root.ContentBoxSize()
		log("  root box=%dx%d", rw, rh)
	}
}

const (
	// 最小尺寸要容纳：顶栏固定控件 + 500px 宽的对话框（记事/待办/问题/纪实）
	minWinW = 540
	minWinH = 600
)

// sanitizeDate 过滤 date 行为控件的非法读值（空态可能返回 "----/--/--" 之类）
func sanitizeDate(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if _, err := time.ParseInLocation("2006-01-02", v, time.Local); err != nil {
		return ""
	}
	return v
}

// refreshDateFilterInputs 重建顶部日期筛选输入框（date 行为不支持 SetValue，
// 清除/重置只能靠重建元素让 value 属性生效）
func refreshDateFilterInputs() {
	panel := app.gw.GetElementById("date-panel")
	if panel == nil {
		return
	}
	panel.SetHtml(
		`<div class="dp-row"><div class="dp-label">从</div>` +
			`<input id="date-from" type="date" class="dlg-input" style="width:1*;" value="` + app.filter.DateFrom + `"></input></div>` +
			`<div class="dp-row"><div class="dp-label">到</div>` +
			`<input id="date-to" type="date" class="dlg-input" style="width:1*;" value="` + app.filter.DateTo + `"></input></div>` +
			`<div class="dp-row"><div style="width:1*;" class="dp-tip">点框选日期，···· 为不限</div>` +
			`<div class="btn-sm" data-act="date-clear" style="margin-left:0;">清除</div></div>`)
	setInputPreset("date-from", app.filter.DateFrom)
	setInputPreset("date-to", app.filter.DateTo)
}

// ---------- 顶部置顶按钮 ----------

func refreshTopmostBtn() {
	el := app.gw.GetElementById("btn-topmost")
	if el == nil {
		return
	}
	if app.data.Topmost {
		el.AddClass("on")
		el.SetText("已置顶")
	} else {
		el.RemoveClass("on")
		el.SetText("置顶")
	}
}

// ---------- 自绘下拉面板 ----------

// renderFilterPanel 渲染顶部筛选面板：搜索框（常驻）下方的项目列表，
// 支持按项目名搜索过滤；分类/「全部」「未指定」始终可选。
func renderFilterPanel() {
	el := app.gw.GetElementById("filter-list")
	if el == nil {
		return
	}
	kw := strings.TrimSpace(app.filterProjKW)
	var sb strings.Builder
	allCls := ""
	if app.data.FilterProjectID == FilterAll {
		allCls = " on"
	}
	sb.WriteString(`<div class="fp-item` + allCls + `" data-act="filter-pick" data-id="all"><span class="sub">全部项目</span></div>`)
	noneCls := ""
	if app.data.FilterProjectID == "" {
		noneCls = " on"
	}
	sb.WriteString(`<div class="fp-item` + noneCls + `" data-act="filter-pick" data-id=""><span class="sub">未指定</span></div>`)
	cats := append([]Category{}, app.data.Categories...)
	sort.SliceStable(cats, func(i, j int) bool { return naturalLess(cats[i].Name, cats[j].Name) })
	hits := 0
	for _, c := range cats {
		ps := append([]Project{}, c.Projects...)
		sort.SliceStable(ps, func(i, j int) bool { return naturalLess(ps[i].Name, ps[j].Name) })
		if kw != "" {
			kept := ps[:0]
			for _, p := range ps {
				if app.data.projectMatches(p.ID, p.Name, kw) {
					kept = append(kept, p)
				}
			}
			ps = kept
			if len(ps) == 0 && !containsFold(c.Name, kw) {
				continue
			}
		}
		if len(ps) == 0 {
			continue
		}
		catCls := ""
		if cid, ok := parseCatFilter(app.data.FilterProjectID); ok && cid == c.ID {
			catCls = " on"
		}
		sb.WriteString(`<div class="fp-cat` + catCls + `" data-act="filter-pick" data-id="` + html.EscapeString(catFilterValue(c.ID)) +
			`"><span class="sub">` + html.EscapeString(c.Name) + `（全部）</span></div>`)
		for _, p := range ps {
			hits++
			cls := ""
			if p.ID == app.data.FilterProjectID {
				cls = " on"
			}
			sb.WriteString(`<div class="fp-item` + cls + `" data-act="filter-pick" data-id="` + html.EscapeString(p.ID) +
				`"><span class="sub">` + html.EscapeString(p.Name) + `</span></div>`)
		}
	}
	if kw != "" && hits == 0 {
		sb.WriteString(`<div class="fp-empty">无匹配项目</div>`)
	}
	el.SetHtml(sb.String())
	updateFilterLabel()
}

// clearFilterProjSearch 清空项目栏搜索词并刷新
func clearFilterProjSearch() {
	app.filterProjKW = ""
	setProgValue("filter-search", "")
	renderFilterPanel()
}

func updateFilterLabel() {
	el := app.gw.GetElementById("filter-label")
	if el == nil {
		return
	}
	switch {
	case app.data.FilterProjectID == FilterAll:
		el.SetText("全部项目")
	default:
		if cid, ok := parseCatFilter(app.data.FilterProjectID); ok {
			el.SetText("[分类] " + app.data.categoryName(cid))
		} else {
			el.SetText(app.data.projectName(app.data.FilterProjectID))
		}
	}
}

// ---------- 列表渲染 ----------

func renderList() {
	// 列表重建后 DOM 元素全部换新，武装态必须重置（否则 armedEl 成为野指针）
	app.armedID = ""
	app.armedEl = nil
	if app.armedTmr != 0 {
		app.gw.KillTimer(app.armedTmr)
		app.armedTmr = 0
	}
	listEl := app.gw.GetElementById("list")
	if listEl == nil {
		return
	}
	f := app.filter
	f.RecentDays = app.recentDays
	rows := app.data.visibleRows(app.allItems(), app.probs, f)

	// 分区：置顶记事 | 待办区（每行两个）| 试机记录区（按天分组）
	var pinnedRows, todoRows, feedRows []ListRow
	for _, r := range rows {
		switch {
		case isPinnedNoteRow(r):
			pinnedRows = append(pinnedRows, r)
		case isPendingTodoRow(r):
			todoRows = append(todoRows, r)
		default:
			feedRows = append(feedRows, r)
		}
	}

	var sb strings.Builder
	for _, r := range pinnedRows {
		sb.WriteString(renderCard(*r.Item))
	}

	// 待办区：非紧急默认收纳，其余每行两个网格
	if len(todoRows) > 0 {
		if len(pinnedRows) > 0 || len(feedRows) > 0 {
			sb.WriteString(`<div class="zone-head">待办</div>`)
		}
		var head, low []ListRow
		for _, r := range todoRows {
			if isLowPendingRow(r) {
				low = append(low, r)
			} else {
				head = append(head, r)
			}
		}
		if len(head) > 0 {
			sb.WriteString(renderTodoGrid(head))
		}
		if len(low) > 0 {
			sb.WriteString(renderLowBar(len(low)))
			if app.lowOpen {
				sb.WriteString(renderTodoGrid(low))
			}
		}
	}

	// 试机记录区：按天分组，天与天之间分割线 + 提示文案
	if len(feedRows) > 0 {
		if len(pinnedRows) > 0 || len(todoRows) > 0 {
			sb.WriteString(`<div class="zone-head">试机记录</div>`)
		}
		lastDay := ""
		for _, r := range feedRows {
			t := rowTime(r)
			dk := dayKey(t)
			if dk != lastDay {
				sb.WriteString(renderDaySep(t, lastDay == ""))
				lastDay = dk
			}
			if r.Prob != nil {
				sb.WriteString(renderProbCard(*r.Prob))
			} else {
				sb.WriteString(renderCard(*r.Item))
			}
		}
	}

	// 更早的记录点「加载更早」逐段展开（每次多显示 5 天）
	hiddenFeed := countHiddenFeed(f, len(feedRows))
	if hiddenFeed > 0 {
		sb.WriteString(`<div class="load-more" data-act="load-more">加载更早记录（还有 ` +
			fmt.Sprint(hiddenFeed) + ` 条）</div>`)
	}
	listEl.SetHtml(sb.String())

	// 统计按当前窗口内全部命中行计算（含被收纳的非紧急待办）
	allTotal := len(rows)
	pendingCount, probCount := len(todoRows), 0
	for _, r := range rows {
		if r.Prob != nil && !r.Prob.Resolved {
			probCount++
		}
	}

	// 空状态：仅当确实没有任何内容时才提示
	tip := app.gw.GetElementById("empty-tip")
	if tip != nil {
		if allTotal == 0 {
			switch {
			case app.filter.Active():
				tip.SetText("没有符合条件的记录")
			case hiddenFeed > 0:
				tip.SetText("最近 " + fmt.Sprint(app.recentDays) + " 天没有记录，点下方按钮加载更早")
			default:
				tip.SetHtml("暂无记录<br/>点击下方按钮开始记录吧")
			}
			tip.Show()
		} else {
			tip.Hide()
		}
	}

	// 日期按钮高亮：日期筛选生效或面板打开时常亮
	if btn := app.gw.GetElementById("btn-date"); btn != nil {
		dateActive := app.filter.DateFrom != "" || app.filter.DateTo != ""
		panelOpen := false
		if el := app.gw.GetElementById("date-panel"); el != nil && panelVisible(el) {
			panelOpen = true
		}
		if dateActive || panelOpen {
			btn.AddClass("on")
		} else {
			btn.RemoveClass("on")
		}
	}

	// 计数
	count := app.gw.GetElementById("count-tip")
	if count != nil {
		if app.filter.Active() {
			count.SetText(fmt.Sprintf("搜索到 %d 条", allTotal))
		} else {
			count.SetText(fmt.Sprintf("待办%d·问题%d·共%d条", pendingCount, probCount, allTotal))
		}
	}
}

// renderTodoGrid 待办区网格：每行两个卡片，奇数补空位保持对齐
func renderTodoGrid(rows []ListRow) string {
	var sb strings.Builder
	for i := 0; i < len(rows); i += 2 {
		sb.WriteString(`<div class="todo-row">`)
		sb.WriteString(`<div class="todo-cell">` + renderCard(*rows[i].Item) + `</div>`)
		if i+1 < len(rows) {
			sb.WriteString(`<div class="todo-cell">` + renderCard(*rows[i+1].Item) + `</div>`)
		} else {
			sb.WriteString(`<div class="todo-cell"></div>`)
		}
		sb.WriteString(`</div>`)
	}
	return sb.String()
}

// renderDaySep 天与天之间的分割线 + 提示文案：
// 首天标「今天/昨天/日期 的记录」，其余标注「下方是另一天的内容（日期）」
func renderDaySep(t time.Time, first bool) string {
	txt := "↓ 下方是另一天的内容（" + dayLabel(t) + "）"
	if first {
		txt = "▼ " + dayLabel(t) + " 的记录"
	}
	return `<div class="day-sep"><div class="day-sep-line"></div>` +
		`<div class="day-sep-txt">` + html.EscapeString(txt) + `</div>` +
		`<div class="day-sep-line"></div></div>`
}

// countHiddenFeed 被时间窗口挡在更早日期的记录条数（「加载更早」按钮用）
func countHiddenFeed(f FilterState, shownFeed int) int {
	fAll := f
	fAll.RecentDays = -1
	n := 0
	for _, r := range app.data.visibleRows(app.allItems(), app.probs, fAll) {
		if isFeedRow(r) {
			n++
		}
	}
	if n > shownFeed {
		return n - shownFeed
	}
	return 0
}

func renderCard(it Item) string {
	// 多项目标记：每个项目一个标签
	projTags := ""
	for _, pid := range itemProjects(it) {
		projTags += `<span class="tag">` + html.EscapeString(app.data.projectName(pid)) + `</span>`
	}
	cls := "card"
	chk := ""
	extraTags := ""

	if it.Type == TypeTodo {
		if it.Done {
			cls += " todo-done"
			extraTags += `<span class="tag done-tag">已完成</span>`
			// 完成时间参与排序，也在这里显示出来
			if !it.DoneTime.IsZero() {
				extraTags += `<span class="tag done-tag">完成 ` + fmtTime(it.DoneTime) + `</span>`
			}
		} else {
			// 紧急浅红 / 普通浅黄 / 非紧急浅灰
			cls += " todo-pending"
			if it.Urgent {
				cls += " urgent"
				extraTags += `<span class="tag urgent-tag">紧急</span>`
			} else if it.Low {
				cls += " low"
				extraTags += `<span class="tag low-tag">非紧急</span>`
			}
			if it.DueDate != "" {
				if dd, err := time.ParseInLocation("2006-01-02", it.DueDate, time.Local); err == nil {
					extraTags += `<span class="tag due-tag">截止 ` + html.EscapeString(it.DueDate)
					if it.DueTime != "" {
						extraTags += ` ` + html.EscapeString(it.DueTime)
					}
					extraTags += ` ` + weekName(dd) + `</span>`
				} else {
					extraTags += `<span class="tag due-tag">截止 ` + html.EscapeString(it.DueDate) + `</span>`
				}
			}
		}
		checked := ""
		if it.Done {
			checked = " on"
		}
		chk = `<div class="chk` + checked + `" data-act="todo-toggle" data-id="` + it.ID + `">√</div>`
	} else if it.Pinned {
		cls += " note-pinned"
		extraTags += `<span class="tag pin-tag">置顶</span>`
	}

	textCls := "card-text"
	if it.Type == TypeTodo && it.Done {
		textCls += " done-text"
	}

	linksHTML := ""
	if len(it.Links) > 0 {
		linksHTML = `<div class="card-links">`
		for _, l := range it.Links {
			linksHTML += `<div class="link-row"><div class="link-item" data-act="link-open" data-path="` +
				html.EscapeString(l.Path) + `">` +
				html.EscapeString(l.Name) + `</div><span class="link-arrow">↗ 文件夹</span></div>`
		}
		linksHTML += `</div>`
	}

	// 子待办清单
	subHTML := ""
	if len(it.SubTodos) > 0 {
		subHTML = `<div class="sub-list">`
		for _, st := range it.SubTodos {
			subCls := "sub-chk"
			subTxt := "sub-text"
			if st.Done {
				subCls += " on"
				subTxt += " done"
			}
			subHTML += `<div class="sub-row">` +
				`<div class="` + subCls + `" data-act="sub-toggle" data-id="` + it.ID + `" data-sub="` + st.ID + `">√</div>` +
				`<div class="` + subTxt + `">` + html.EscapeString(st.Text) + `</div>` +
				`<div class="sub-del" data-act="sub-del" data-id="` + it.ID + `" data-sub="` + st.ID + `">×</div></div>`
		}
		subHTML += `</div>`
	}

	// 置顶按钮（记事）
	pinHTML := ""
	if it.Type == TypeNote {
		pinCls := "btn-pin"
		if it.Pinned {
			pinCls += " on"
		}
		pinHTML = `<div class="` + pinCls + `" data-act="note-pin" data-id="` + it.ID + `">钉</div>`
	}

	// 卡片分三层：勾选框 | 可点击编辑区（文本+元信息+子待办） | 链接区 | 置顶
	// 删除改右键菜单（见 onMouseDown），卡片上不再放删除按钮
	return `<div class="` + cls + `" data-id="` + it.ID + `">` + chk +
		`<div class="card-mid">` +
		`<div class="card-click" data-act="card-edit" data-id="` + it.ID + `">` +
		`<div class="` + textCls + `">` + renderText(it.Text) + `</div>` +
		`<div class="card-meta">` +
		projTags +
		extraTags +
		`<span class="meta-time">` + fmtTime(it.Time) + `</span>` +
		`</div></div>` +
		subHTML +
		linksHTML +
		`</div>` +
		pinHTML +
		`</div>`
}

// renderText 输出正文：先转义 HTML，再把 **文字** 渲染成加粗，用于高亮提醒内容
func renderText(s string) string {
	esc := html.EscapeString(s)
	var b strings.Builder
	for {
		i := strings.Index(esc, "**")
		if i < 0 {
			b.WriteString(esc)
			return b.String()
		}
		b.WriteString(esc[:i])
		rest := esc[i+2:]
		j := strings.Index(rest, "**")
		if j < 0 { // 没有配对的结束标记，原样输出
			b.WriteString(esc[i:])
			return b.String()
		}
		inner := rest[:j]
		if strings.TrimSpace(inner) == "" { // 空标记不转义，避免误吞
			b.WriteString("**")
			esc = rest
			continue
		}
		b.WriteString(`<b class="hl">` + inner + `</b>`)
		esc = rest[j+2:]
	}
}

// renderLowBar 非紧急待办的收纳条（默认收纳，点击展开全部）
func renderLowBar(count int) string {
	if app.lowOpen {
		return `<div class="low-bar" data-act="low-toggle">非紧急待办 ` + fmt.Sprint(count) +
			` 条 ▼ 点击收起</div>`
	}
	return `<div class="low-bar" data-act="low-toggle">非紧急待办 ` + fmt.Sprint(count) +
		` 条 ▶ 点击展开</div>`
}

// renderProbCard 问题排查卡片：默认折叠，点击头部展开/收起
func renderProbCard(p Problem) string {
	cls := "card prob-card"
	expanded := app.probExpanded[p.ID]
	if p.Resolved {
		cls += " prob-resolved"
	}
	if expanded {
		cls += " prob-open"
	}

	chkCls := "chk prob-chk"
	if p.Resolved {
		chkCls += " on"
	}
	chk := `<div class="` + chkCls + `" data-act="prob-resolve" data-id="` + p.ID + `">√</div>`

	tags := `<span class="tag prob-tag">问题排查</span>`
	for _, pid := range probProjects(p) {
		tags += `<span class="tag">` + html.EscapeString(app.data.projectName(pid)) + `</span>`
	}
	if p.Resolved {
		tags += `<span class="tag done-tag">已解决</span>`
	} else {
		tags += `<span class="tag urgent-tag">未解决</span>`
	}

	arrow := "▶ 展开"
	if expanded {
		arrow = "▼ 收起"
	}

	head := `<div class="card-click" data-act="prob-expand" data-id="` + p.ID + `">` +
		`<div class="card-text">` + renderText(p.Name) + `</div>` +
		`<div class="card-meta">` + tags +
		`<span class="meta-time">` + fmtTime(p.Time) + `</span>` +
		`<span class="expand-arrow">` + arrow + `</span>` +
		`</div></div>`

	body := ""
	if expanded {
		var sb strings.Builder
		// 档案
		sb.WriteString(`<div class="prob-sec"><div class="prob-sec-title">档案</div>`)
		if p.ProfileText != "" {
			sb.WriteString(`<div class="prob-text">` + renderText(p.ProfileText) + `</div>`)
		}
		for _, l := range p.ProfileLinks {
			sb.WriteString(`<div class="link-row"><div class="link-item" data-act="link-open" data-path="` +
				html.EscapeString(l.Path) + `">` +
				html.EscapeString(l.Name) + `</div><span class="link-arrow">↗ 文件夹</span></div>`)
		}
		if p.ProfileText == "" && len(p.ProfileLinks) == 0 {
			sb.WriteString(`<div class="prob-empty">暂无档案，点「编辑」添加</div>`)
		}
		sb.WriteString(`</div>`)
		// 纪实
		sb.WriteString(`<div class="prob-sec"><div class="prob-sec-title">纪实` +
			`<div class="btn-sm" data-act="log-add" data-id="` + p.ID + `" style="height:24px;padding:0 10px;margin-left:10px;">+ 添加</div></div>`)
		if len(p.Log) == 0 {
			sb.WriteString(`<div class="prob-empty">暂无纪实</div>`)
		}
		for _, e := range p.Log {
			sb.WriteString(`<div class="prob-log-row">` +
				`<div class="prob-log-head"><span class="prob-log-time">` + fmtTime(e.Time) + `</span>` +
				`<div class="prob-log-acts">` +
				`<div class="btn-sm" data-act="log-edit" data-id="` + p.ID + `" data-entry="` + e.ID + `" style="height:22px;padding:0 8px;font-size:9pt;">编辑</div>` +
				`<div class="btn-sm danger" data-act="log-del" data-id="` + p.ID + `" data-entry="` + e.ID + `" style="height:22px;padding:0 8px;font-size:9pt;margin-left:6px;">删除</div>` +
				`</div></div>` +
				`<div class="prob-text">` + renderText(e.Text) + `</div>`)
			for _, l := range e.Links {
				sb.WriteString(`<div class="link-row"><div class="link-item" data-act="link-open" data-path="` +
					html.EscapeString(l.Path) + `">` +
					html.EscapeString(l.Name) + `</div><span class="link-arrow">↗ 文件夹</span></div>`)
			}
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
		// 操作行
		sb.WriteString(`<div class="prob-acts">` +
			`<div class="btn-sm primary" data-act="prob-edit" data-id="` + p.ID + `" style="height:26px;">编辑档案</div>` +
			`</div>`)
		body = `<div class="prob-body">` + sb.String() + `</div>`
	}

	return `<div class="` + cls + `" data-id="` + p.ID + `">` + chk +
		`<div class="card-mid">` + head + body + `</div>` +
		`</div>`
}

// ---------- 事件分发 ----------

func attrOrParent(elem *gohl.Element, key string) (string, bool) {
	if v, ok := elem.Attr(key); ok {
		return v, true
	}
	if p := elem.SelectParent("[" + key + "]"); p != nil {
		if v, ok := p.Attr(key); ok {
			return v, true
		}
	}
	return "", false
}

func hideAllPanels() {
	hideDialog("filter-panel")
	hideDialog("date-panel")
	if btn := app.gw.GetElementById("btn-date"); btn != nil {
		btn.RemoveClass("on")
	}
	hideDialog("note-cat-panel")
	hideDialog("note-proj-panel")
	hideDialog("todo-cat-panel")
	hideDialog("todo-proj-panel")
	hideDialog("prob-cat-panel")
	hideDialog("prob-proj-panel")
}

func onClick(elem *gohl.Element) {
	if elem == nil {
		return
	}
	act, ok := attrOrParent(elem, "data-act")
	if !ok {
		return
	}
	// 点击其他操作时解除删除武装态（点同一个删除按钮除外，交由 handleDelete 处理）
	if act != "del" && app.armedID != "" {
		disarmDelete()
	}
	switch act {
	case "link-open":
		// 打开链接所在的文件夹（不是打开文件本身）。用显式按钮处理，
		// 不走 <a href> 超链接默认行为，避免被 HTMLayout 直接用关联程序打开文件
		// 及其中文路径乱码。
		if p, ok := attrOrParent(elem, "data-path"); ok {
			openPath(uintptr(app.gw.GetHwnd()), p)
		}

	case "win-min":
		// 最小化到右下角托盘（无边框窗口本就不显示在任务栏）
		hideToTray()

	case "win-max":
		// 最大化 / 还原；最大化时窗口区域改直角，圆角会切掉四角
		hwnd := uintptr(app.gw.GetHwnd())
		if isZoomed(hwnd) {
			app.gw.Restore()
			applyRoundedRegion(hwnd, 12)
		} else {
			app.gw.Maximize()
			clearWindowRegion(hwnd)
		}

	case "win-close":
		exitApp()

	case "toggle-topmost":
		app.data.Topmost = !app.data.Topmost
		setTopmost(uintptr(app.gw.GetHwnd()), app.data.Topmost)
		refreshTopmostBtn()
		save()

	case "add-note":
		hideAllPanels()
		openNoteDialog("")

	case "add-todo":
		hideAllPanels()
		openTodoDialog("")

	case "card-edit":
		// 点击卡片进入编辑（记事/待办均可改内容、时间、项目）
		if id, ok := attrOrParent(elem, "data-id"); ok {
			if it := app.findItem(id); it != nil {
				hideAllPanels()
				if it.Type == TypeTodo {
					openTodoDialog(id)
				} else {
					openNoteDialog(id)
				}
			}
		}

	case "todo-toggle":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			toggleTodo(id)
		}

	case "note-pin":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			togglePin(id)
		}

	case "sub-toggle":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			subID, _ := attrOrParent(elem, "data-sub")
			toggleSubTodo(id, subID)
		}

	case "sub-del":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			subID, _ := attrOrParent(elem, "data-sub")
			handleSubDel(id, subID, elem)
		}

	case "sub-add":
		addDraftSub()

	case "sub-del-draft":
		if subID, ok := attrOrParent(elem, "data-sub"); ok {
			subs := app.draftSubs[:0]
			for _, st := range app.draftSubs {
				if st.ID != subID {
					subs = append(subs, st)
				}
			}
			app.draftSubs = subs
			renderDraftSubs()
		}

	case "add-prob":
		hideAllPanels()
		openProbDialog("")

	case "prob-resolve":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			toggleProbResolved(id)
		}

	case "prob-expand":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			app.probExpanded[id] = !app.probExpanded[id]
			renderList()
		}

	case "prob-edit":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			hideAllPanels()
			openProbDialog(id)
		}

	case "prob-del":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			handleProbDelete(id, elem)
		}

	case "prob-save":
		saveProbDialog()

	case "prob-cancel", "log-cancel":
		hideAllPanels()
		hideDialog("dlg-prob-mask")
		hideDialog("dlg-log-mask")

	case "log-add":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			openLogDialog(id, "")
		}

	case "log-edit":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			entryID, _ := attrOrParent(elem, "data-entry")
			openLogDialog(id, entryID)
		}

	case "log-del":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			entryID, _ := attrOrParent(elem, "data-entry")
			handleLogDelete(id, entryID, elem)
		}

	case "log-save":
		saveLogDialog()

		case "load-more":
			app.recentDays += loadMoreStepDays
			renderList()

	case "note-cancel", "todo-cancel", "proj-close":
		hideAllPanels()
		hideDialog("dlg-note-mask")
		hideDialog("dlg-todo-mask")
		hideDialog("dlg-proj-mask")

	case "note-save":
		saveNoteDialog()

	case "todo-save":
		saveTodoDialog()

	case "pick-file":
		p := pickFile(uintptr(app.gw.GetHwnd()))
		if p != "" {
			addDraftLinkTo(currentLinkTarget(), p)
		}

	case "pick-folder":
		p := pickFolder(uintptr(app.gw.GetHwnd()))
		if p != "" {
			addDraftLinkTo(currentLinkTarget(), p)
		}

	case "link-del":
		removeDraftLinkFrom(currentLinkTarget(), elem)

	case "prob-link-del":
		removeDraftLinkFrom("prob", elem)

	case "log-link-del":
		removeDraftLinkFrom("log", elem)

	case "todo-urgent":
		setTodoPriority("urgent")

	case "todo-normal":
		setTodoPriority("normal")

	case "todo-low":
		setTodoPriority("low")

	case "low-toggle":
		// 非紧急待办：默认收纳，点击展开全部 / 再点收起
		app.lowOpen = !app.lowOpen
		renderList()

	case "note-pin-on":
		app.gw.GetElementById("note-pin-on").AddClass("on")
		app.gw.GetElementById("note-pin-off").RemoveClass("on")

	case "note-pin-off":
		app.gw.GetElementById("note-pin-off").AddClass("on")
		app.gw.GetElementById("note-pin-on").RemoveClass("on")

	case "manage-proj":
		hideAllPanels()
		openProjDialog()

	case "proj-add-new":
		createProjectFile(
			strings.TrimSpace(app.gw.GetElementValue("proj-new-cat")),
			strings.TrimSpace(app.gw.GetElementValue("proj-new-proj")),
		)

	case "proj-add-item":
		// 分类名在 data-cat 上，行内输入框 ID 在 data-input 上
		catName, ok1 := attrOrParent(elem, "data-cat")
		inputID, ok2 := attrOrParent(elem, "data-input")
		if ok1 && ok2 {
			createProjectFile(catName, strings.TrimSpace(app.gw.GetElementValue(inputID)))
		}

	case "proj-alias-edit":
		// 打开该项目的别名编辑行（再点一次收起）
		if id, ok := attrOrParent(elem, "data-id"); ok {
			if app.editingAliasPID == id {
				app.editingAliasPID = ""
			} else {
				app.editingAliasPID = id
			}
			renderProjDialog()
		}

	case "proj-alias-save":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			raw := app.gw.GetElementValue("proj-alias-input")
			app.data.setProjectAliases(id, parseAliasList(raw))
			app.editingAliasPID = ""
			save()
			renderProjDialog()
			renderFilterPanel() // 别名参与项目栏搜索
		}

	case "proj-alias-cancel":
		app.editingAliasPID = ""
		renderProjDialog()

	case "del":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			handleDelete(id, elem)
		}

	case "search-clear":
		app.filter.Keyword = ""
		setProgValue("search-input", "")
		app.recentDays = defaultRecentDays
		renderList()

	case "date-clear":
		app.filter.DateFrom = ""
		app.filter.DateTo = ""
		refreshDateFilterInputs() // date 行为不支持 SetValue，重建元素
		app.recentDays = defaultRecentDays
		renderList()

	case "filter-search-clear":
		clearFilterProjSearch()

	case "filter-toggle":
		rescanProjects() // 打开筛选面板时同步磁盘上的项目文件
		if !panelVisible(app.gw.GetElementById("filter-panel")) {
			clearFilterProjSearch() // 每次打开从完整列表开始
		}
		renderFilterPanel()
		togglePanel("filter-panel")

	case "date-toggle":
		togglePanel("date-panel")
		renderList() // 同步日期按钮高亮

	case "filter-pick":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			app.data.FilterProjectID = id
			if id == FilterAll {
				app.filter = FilterState{}
				setProgValue("search-input", "")
				refreshDateFilterInputs()
			}
			app.recentDays = defaultRecentDays
			save()
			renderFilterPanel()
			hideDialog("filter-panel")
			renderList()
		}

	case "note-cat-toggle":
		app.gw.GetElementById("note-cat-panel").SetHtml(buildCatPanelHTML(app.noteCatID))
		togglePanel("note-cat-panel")

	case "todo-cat-toggle":
		app.gw.GetElementById("todo-cat-panel").SetHtml(buildCatPanelHTML(app.todoCatID))
		togglePanel("todo-cat-panel")

	case "prob-cat-toggle":
		app.gw.GetElementById("prob-cat-panel").SetHtml(buildCatPanelHTML(app.probCatID))
		togglePanel("prob-cat-panel")

	case "note-proj-open":
		toggleProjPanel("note")

	case "todo-proj-open":
		toggleProjPanel("todo")

	case "prob-proj-open":
		toggleProjPanel("prob")

	case "cat-pick":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			notePanel := app.gw.GetElementById("note-cat-panel")
			if notePanel != nil && panelVisible(notePanel) {
				app.noteCatID = id
				app.gw.GetElementById("note-cat-label").SetText(catDisplayName(id))
				hideDialog("note-cat-panel")
			}
			todoPanel := app.gw.GetElementById("todo-cat-panel")
			if todoPanel != nil && panelVisible(todoPanel) {
				app.todoCatID = id
				app.gw.GetElementById("todo-cat-label").SetText(catDisplayName(id))
				hideDialog("todo-cat-panel")
			}
			probPanel := app.gw.GetElementById("prob-cat-panel")
			if probPanel != nil && panelVisible(probPanel) {
				app.probCatID = id
				app.gw.GetElementById("prob-cat-label").SetText(catDisplayName(id))
				hideDialog("prob-cat-panel")
			}
		}

	case "proj-pick":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			kind := visibleProjKind()
			if kind == "" {
				return
			}
			// 点选即确认：加入/移出选择后面板立即收起，无需再点「完成」；
			// 需要多选时再按 ▼（或继续输入）展开继续点
			if id == "" {
				setProjIDs(kind, nil) // 选「未指定」等于清空
			} else {
				toggleProjID(kind, id)
			}
			setProgValue(kind+"-proj-input", "")
			hideDialog(kind + "-proj-panel")
			renderProjSelection(kind)
		}

	case "proj-unpick":
		if id, ok := attrOrParent(elem, "data-id"); ok {
			kind := visibleProjKind()
			if kind == "" {
				return
			}
			toggleProjID(kind, id)
			renderProjSelection(kind)
			el := app.gw.GetElementById(kind + "-proj-panel")
			if el != nil && panelVisible(el) {
				el.SetHtml(buildProjOptionsHTML(catIDOf(kind), "", projIDsOf(kind)))
			}
		}
	}
}

// visibleProjKind 判断当前哪个对话框的项目面板在显示
func visibleProjKind() string {
	for _, kind := range []string{"note", "todo", "prob"} {
		if el := app.gw.GetElementById(kind + "-proj-sel"); el != nil {
			if p := app.gw.GetElementById(kind + "-proj-panel"); p != nil && panelVisible(p) {
				return kind
			}
		}
	}
	// 面板已关闭时，按哪个对话框打开着来判断
	for _, kind := range []string{"note", "todo", "prob"} {
		if el := app.gw.GetElementById("dlg-" + kind + "-mask"); el != nil && panelVisible(el) {
			return kind
		}
	}
	return ""
}

func catIDOf(kind string) string {
	switch kind {
	case "todo":
		return app.todoCatID
	case "prob":
		return app.probCatID
	default:
		return app.noteCatID
	}
}

func catDisplayName(cid string) string {
	if cid == "" {
		return "全部分类"
	}
	return app.data.categoryName(cid)
}

// panelVisible 判断面板是否处于显示状态（class 含 show）
func panelVisible(el *gohl.Element) bool {
	if el == nil {
		return false
	}
	if cls, ok := el.Attr("class"); ok {
		for _, c := range strings.Fields(cls) {
			if c == "show" {
				return true
			}
		}
	}
	return false
}

func togglePanel(id string) {
	el := app.gw.GetElementById(id)
	if el == nil {
		return
	}
	if panelVisible(el) {
		el.Hide()
	} else {
		el.Show()
		positionPanel(id)
	}
}

// panelAnchors 顶部弹出面板 → 锚点按钮（面板贴在按钮正下方，窗口缩放后也跟随）
var panelAnchors = map[string]string{
	"filter-panel": "filter-btn",
	"date-panel":   "btn-date",
}

// positionPanel 把面板定位到锚点按钮左下角，并保证不超出窗口右边界
func positionPanel(id string) {
	anchorID, ok := panelAnchors[id]
	if !ok {
		return
	}
	anchor := app.gw.GetElementById(anchorID)
	el := app.gw.GetElementById(id)
	if anchor == nil || el == nil {
		return
	}
	al, _, ar, ab := anchor.ContentViewBox()
	if ar <= al {
		return
	}
	pw, ph := el.ContentBoxSize()
	if pw <= 0 {
		return
	}
	cw, ch := getClientSize(uintptr(app.gw.GetHwnd()))
	left := al
	// 右侧溢出时往左回移
	if cw > 0 && left+pw > cw-6 {
		left = cw - 6 - pw
	}
	if left < 6 {
		left = 6
	}
	top := ab + 4
	// 下方溢出时往上翻（贴在按钮上方）
	if ch > 0 && ph > 0 && top+ph > ch-6 {
		if alt := ab - 4 - ph; alt >= 6 {
			top = alt
		} else {
			top = ch - 6 - ph
			if top < 6 {
				top = 6
			}
		}
	}
	el.SetStyle("left", fmt.Sprint(left)+"px")
	el.SetStyle("top", fmt.Sprint(top)+"px")
	el.Update(true, true, true, true, true)
}

// repositionVisiblePanels 窗口尺寸变化后重新定位仍可见的顶部面板
func repositionVisiblePanels() {
	for id := range panelAnchors {
		el := app.gw.GetElementById(id)
		if el != nil && panelVisible(el) {
			positionPanel(id)
		}
	}
}

func projectDisplayName(pid string) string {
	if pid == "" {
		return "未指定"
	}
	return app.data.projectName(pid)
}

// ---------- 对话框项目选择（分类 + 项目搜索） ----------

// buildCatPanelHTML 分类选择面板："全部分类" + 各分类（含项目数）
func buildCatPanelHTML(selected string) string {
	var sb strings.Builder
	cls := ""
	if selected == "" {
		cls = " on"
	}
	sb.WriteString(`<div class="dd-item` + cls + `" data-act="cat-pick" data-id=""><span class="sub">全部分类</span></div>`)
	cats := append([]Category{}, app.data.Categories...)
	sort.SliceStable(cats, func(i, j int) bool { return naturalLess(cats[i].Name, cats[j].Name) })
	for _, c := range cats {
		cls := ""
		if c.ID == selected {
			cls = " on"
		}
		sb.WriteString(`<div class="dd-item` + cls + `" data-act="cat-pick" data-id="` + html.EscapeString(c.ID) +
			`"><span class="sub">` + html.EscapeString(c.Name) + `（` + fmt.Sprint(len(c.Projects)) + `）</span></div>`)
	}
	return sb.String()
}

// projIDsOf / setProjIDs 取/设某类对话框当前选中的项目（可多选）
func projIDsOf(kind string) []string {
	switch kind {
	case "todo":
		return app.todoProjIDs
	case "prob":
		return app.probProjIDs
	default:
		return app.noteProjIDs
	}
}

func setProjIDs(kind string, ids []string) {
	switch kind {
	case "todo":
		app.todoProjIDs = ids
	case "prob":
		app.probProjIDs = ids
	default:
		app.noteProjIDs = ids
	}
}

// toggleProjID 切换某个项目的选中状态（多选）
func toggleProjID(kind, id string) {
	cur := projIDsOf(kind)
	out := []string{}
	has := false
	for _, p := range cur {
		if p == id {
			has = true
			continue
		}
		out = append(out, p)
	}
	if !has {
		out = append(out, id)
	}
	setProjIDs(kind, out)
}

// renderProjSelection 渲染对话框里「已选项目」的标签行（点 × 移除）
func renderProjSelection(kind string) {
	el := app.gw.GetElementById(kind + "-proj-sel")
	if el == nil {
		return
	}
	ids := projIDsOf(kind)
	if len(ids) == 0 {
		el.SetHtml(`<div class="proj-sel-empty">未指定项目（可多选）</div>`)
		return
	}
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(`<div class="proj-chip"><span class="proj-chip-name">` +
			html.EscapeString(app.data.projectName(id)) + `</span>` +
			`<div class="proj-chip-del" data-act="proj-unpick" data-id="` + html.EscapeString(id) + `">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

// buildProjOptionsHTML 项目选项列表：filter 非空时跨全部分类模糊匹配；
// 否则 catID 非空时只列该分类。已选项带 on 与 ✓ 标记（支持多选）。
func buildProjOptionsHTML(catID, filter string, selected []string) string {
	var sb strings.Builder
	sel := map[string]bool{}
	for _, id := range selected {
		sel[id] = true
	}
	// 顶部提示条：点选即确认（可多选，再按 ▾ 继续添加）
	sb.WriteString(`<div class="dd-head"><span class="dd-head-tip">已选 ` + fmt.Sprint(len(selected)) +
		` 项 · 点选即确认，可多选</span></div>`)
	noneCls := ""
	if len(selected) == 0 {
		noneCls = " on"
	}
	sb.WriteString(`<div class="dd-item` + noneCls + `" data-act="proj-pick" data-id=""><span class="sub">未指定</span></div>`)
	cats := append([]Category{}, app.data.Categories...)
	sort.SliceStable(cats, func(i, j int) bool { return naturalLess(cats[i].Name, cats[j].Name) })
	if catID != "" && filter == "" {
		cats = filterCats(cats, catID)
	}
	found := 0
	for _, c := range cats {
		ps := append([]Project{}, c.Projects...)
		sort.SliceStable(ps, func(i, j int) bool { return naturalLess(ps[i].Name, ps[j].Name) })
		if filter != "" {
			kept := ps[:0]
			for _, p := range ps {
				if app.data.projectMatches(p.ID, p.Name, filter) {
					kept = append(kept, p)
				}
			}
			ps = kept
		}
		if len(ps) == 0 {
			continue
		}
		sb.WriteString(`<div class="dd-cat">` + html.EscapeString(c.Name) + `</div>`)
		for _, p := range ps {
			found++
			cls := ""
			mark := ""
			if sel[p.ID] {
				cls = " on"
				mark = "✓"
			}
			sb.WriteString(`<div class="dd-item` + cls + `" data-act="proj-pick" data-id="` + html.EscapeString(p.ID) +
				`"><span class="dd-check">` + mark + `</span><span class="sub">` + html.EscapeString(p.Name) + `</span></div>`)
		}
	}
	if found == 0 {
		sb.WriteString(`<div class="dd-empty">无匹配项目</div>`)
	}
	return sb.String()
}

func filterCats(cats []Category, catID string) []Category {
	out := cats[:0]
	for _, c := range cats {
		if c.ID == catID {
			out = append(out, c)
		}
	}
	return out
}

// toggleProjPanel ▾ 按钮：面板已开则收起，否则展开（不经输入直接浏览全部项目）
func toggleProjPanel(kind string) {
	if el := app.gw.GetElementById(kind + "-proj-panel"); el != nil && panelVisible(el) {
		el.Hide()
		return
	}
	renderProjOptions(kind)
}

// onProjInputChanged 项目号输入框内容变化：有输入才弹候选面板，清空输入则收起。
// 默认状态下拉一律不展开；▼ 按钮用于不经输入直接浏览全部项目。
func onProjInputChanged(kind string) {
	inputID := kind + "-proj-input"
	text := strings.TrimSpace(app.gw.GetElementValue(inputID))
	if text == "" || text == "未指定" {
		hideDialog(kind + "-proj-panel")
		return
	}
	renderProjOptions(kind)
}

// renderProjOptions 依据输入框内容刷新项目面板并显示（点选即确认，可多选）
func renderProjOptions(kind string) {
	inputID, panelID := "note-proj-input", "note-proj-panel"
	catID := app.noteCatID
	if kind == "todo" {
		inputID, panelID = "todo-proj-input", "todo-proj-panel"
		catID = app.todoCatID
	}
	if kind == "prob" {
		inputID, panelID = "prob-proj-input", "prob-proj-panel"
		catID = app.probCatID
	}
	text := strings.TrimSpace(app.gw.GetElementValue(inputID))
	if text == "未指定" {
		text = ""
	}
	el := app.gw.GetElementById(panelID)
	if el == nil {
		return
	}
	el.SetHtml(buildProjOptionsHTML(catID, text, projIDsOf(kind)))
	el.Show()
	renderProjSelection(kind)
}

// resolveProjectInput 保存时确定项目列表：已点选的 + 输入框里写全的项目名（跨分类精确匹配）
func resolveProjectInput(kind string) []string {
	ids := append([]string{}, projIDsOf(kind)...)
	inputID := "note-proj-input"
	if kind == "todo" {
		inputID = "todo-proj-input"
	}
	if kind == "prob" {
		inputID = "prob-proj-input"
	}
	text := strings.TrimSpace(app.gw.GetElementValue(inputID))
	if text != "" && text != "未指定" {
		if found, ok := app.data.findProjectByName(text); ok {
			dup := false
			for _, id := range ids {
				if id == found {
					dup = true
				}
			}
			if !dup {
				ids = append(ids, found)
			}
		}
	}
	return ids
}

// ---------- 复选框（完成待办） ----------

// toggleTodo 翻转待办完成状态并落盘重绘
func toggleTodo(id string) {
	pid, i := app.findItemPos(id)
	if i < 0 {
		return
	}
	it := &app.items[pid][i]
	if it.Type != TypeTodo {
		return
	}
	it.Done = !it.Done
	if it.Done {
		it.DoneTime = time.Now()
	} else {
		it.DoneTime = time.Time{}
	}
	saveProject(pid)
	renderList() // 完成后按时间倒序混排
}

func onCheckChanged(elem *gohl.Element) {
	id, ok := attrOrParent(elem, "data-id")
	if !ok {
		return
	}
	// 原生 checkbox 的 BUTTON_STATE_CHANGED：直接翻转数据模型（事件派发时 DOM 属性可能尚未同步）
	toggleTodo(id)
}

// ---------- 记事置顶 ----------

// togglePin 翻转记事置顶状态并落盘重绘
func togglePin(id string) {
	pid, i := app.findItemPos(id)
	if i < 0 {
		return
	}
	it := &app.items[pid][i]
	if it.Type != TypeNote {
		return
	}
	it.Pinned = !it.Pinned
	saveProject(pid)
	renderList()
}

// ---------- 子待办 ----------

// toggleSubTodo 翻转子待办完成状态
func toggleSubTodo(id, subID string) {
	pid, i := app.findItemPos(id)
	if i < 0 {
		return
	}
	it := &app.items[pid][i]
	for j := range it.SubTodos {
		if it.SubTodos[j].ID == subID {
			it.SubTodos[j].Done = !it.SubTodos[j].Done
			saveProject(pid)
			renderList()
			return
		}
	}
}

// handleSubDel 两步确认删除子待办
func handleSubDel(id, subID string, elem *gohl.Element) {
	key := "sub:" + id + ":" + subID
	if app.armedID == key {
		disarmDelete()
		pid, i := app.findItemPos(id)
		if i < 0 {
			return
		}
		it := &app.items[pid][i]
		subs := it.SubTodos[:0]
		for _, st := range it.SubTodos {
			if st.ID != subID {
				subs = append(subs, st)
			}
		}
		it.SubTodos = subs
		saveProject(pid)
		renderList()
		return
	}
	disarmDelete()
	app.armedID = key
	app.armedEl = elem
	elem.AddClass("armed")
	elem.SetText("√")
	app.armedTmr = app.gw.SetTimer(10000, func() { disarmDelete() })
}

// addDraftSub 待办对话框：添加一条子待办到暂存列表
func addDraftSub() {
	text := strings.TrimSpace(app.gw.GetElementValue("todo-sub-input"))
	if text == "" {
		return
	}
	app.draftSubs = append(app.draftSubs, SubTodo{ID: newID(), Text: text})
	setProgValue("todo-sub-input", "")
	renderDraftSubs()
}

// renderDraftSubs 渲染待办对话框的子待办暂存列表
func renderDraftSubs() {
	el := app.gw.GetElementById("todo-subs")
	if el == nil {
		return
	}
	if len(app.draftSubs) == 0 {
		el.SetHtml(`<div class="prob-empty">暂无子待办</div>`)
		return
	}
	var sb strings.Builder
	for i, st := range app.draftSubs {
		sb.WriteString(`<div class="sub-edit-row">` +
			`<span class="sub-edit-idx">` + fmt.Sprint(i+1) + `.</span>` +
			`<span class="sub-edit-text">` + html.EscapeString(st.Text) + `</span>` +
			`<div class="link-edit-del" data-act="sub-del-draft" data-sub="` + st.ID + `">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

// ---------- 问题排查 ----------

// toggleProbResolved 翻转问题解决状态
func toggleProbResolved(id string) {
	pr := app.findProb(id)
	if pr == nil {
		return
	}
	pr.Resolved = !pr.Resolved
	if pr.Resolved {
		now := time.Now()
		pr.ResolvedTime = &now
	} else {
		pr.ResolvedTime = nil
	}
	saveProblem(pr)
	renderList()
}

// handleProbDelete 两步确认删除问题（删除其独立存储文件）
func handleProbDelete(id string, elem *gohl.Element) {
	key := "prob:" + id
	if app.armedID == key {
		disarmDelete()
		if pr := app.findProb(id); pr != nil {
			deleteProblemFile(pr.ID)
		}
		app.probExpanded[id] = false
		rescanProjects()
		renderList()
		return
	}
	disarmDelete()
	app.armedID = key
	app.armedEl = elem
	elem.AddClass("armed")
	elem.SetText("√")
	app.armedTmr = app.gw.SetTimer(10000, func() { disarmDelete() })
}

// handleLogDelete 两步确认删除纪实条目
func handleLogDelete(probID, entryID string, elem *gohl.Element) {
	key := "log:" + probID + ":" + entryID
	if app.armedID == key {
		disarmDelete()
		if pr := app.findProb(probID); pr != nil {
			log := pr.Log[:0]
			for _, e := range pr.Log {
				if e.ID != entryID {
					log = append(log, e)
				}
			}
			pr.Log = log
			saveProblem(pr)
		}
		renderList()
		return
	}
	disarmDelete()
	app.armedID = key
	app.armedEl = elem
	elem.AddClass("armed")
	elem.SetText("√")
	app.armedTmr = app.gw.SetTimer(10000, func() { disarmDelete() })
}

// ---------- 删除（两步确认） ----------

func handleDelete(id string, elem *gohl.Element) {
	if app.armedID == id {
		// 第二次点击：执行删除
		disarmDelete()
		if pid, i := app.findItemPos(id); i >= 0 {
			app.items[pid] = append(app.items[pid][:i], app.items[pid][i+1:]...)
			saveProject(pid)
		}
		renderList()
		return
	}
	disarmDelete()
	app.armedID = id
	app.armedEl = elem
	elem.AddClass("armed")
	elem.SetText("√")
	app.armedTmr = app.gw.SetTimer(10000, func() { disarmDelete() })
}

func disarmDelete() {
	if app.armedEl != nil {
		app.armedEl.RemoveClass("armed")
		app.armedEl.SetText("×")
		app.armedEl = nil
	}
	if app.armedTmr != 0 {
		app.gw.KillTimer(app.armedTmr)
		app.armedTmr = 0
	}
	app.armedID = ""
}

// ---------- 右键菜单（删除走二次确认） ----------

const (
	cmEdit    = 10 // 编辑
	cmDelete  = 11 // 删除
	cmPin     = 12 // 记事置顶切换
	cmResolve = 13 // 问题已解决切换
	cmAddLog  = 14 // 问题添加纪实
)

// onMouseDown 鼠标按下：左键按在子界面拖拉把手 → 开始缩放；右键记录目标卡片 ID，等松开后再弹菜单
// （在按下时弹 TrackPopupMenu 会被随后的松开事件立刻关掉）
func onMouseDown(elem *gohl.Element, params *gohl.MouseParams) bool {
	if params == nil {
		return false
	}
	// 左键按在拖拉把手：开始子界面缩放
	if params.ButtonState&gohl.MAIN_MOUSE_BUTTON != 0 {
		if kind, ok := attrOrParent(elem, "data-grip"); ok {
			startDlgDrag(kind)
			return true
		}
	}
	if params.ButtonState&gohl.PROP_MOUSE_BUTTON == 0 {
		return false
	}
	app.pendingRightID = ""
	if elem == nil {
		return false
	}
	card := findCardElement(elem)
	if card == nil {
		return false
	}
	if id, ok := card.Attr("data-id"); ok && id != "" {
		app.pendingRightID = id
	}
	return true
}

// onMouseUp 右键松开：真正弹出卡片菜单
// 必须在本线程（主线程）同步调用：TrackPopupMenu 的模态循环要求当前线程拥有窗口，
// 且 forceForegroundWindow 用的是主线程 ID，放到 goroutine 里会置前失败、菜单立即消失。
func onMouseUp(elem *gohl.Element, params *gohl.MouseParams) bool {
	if app.pendingRightID == "" {
		return false
	}
	id := app.pendingRightID
	app.pendingRightID = ""
	showCardMenu(id)
	return true
}

// findCardElement 从当前元素向上找所属卡片（最多 10 层）
func findCardElement(elem *gohl.Element) *gohl.Element {
	e := elem
	for i := 0; e != nil && i < 10; i++ {
		if cls, ok := e.Attr("class"); ok {
			for _, c := range strings.Fields(cls) {
				if c == "card" {
					return e
				}
			}
		}
		e = e.Parent()
	}
	return nil
}

// showCardMenu 在光标处弹出卡片右键菜单；删除会弹确认框二次确认
func showCardMenu(id string) {
	hwnd := uintptr(app.gw.GetHwnd())
	menu, _, _ := syscall.SyscallN(procCreatePopupMenu.Addr())
	if menu == 0 {
		return
	}
	if pr := app.findProb(id); pr != nil {
		appendMenuItem(menu, mfString, cmAddLog, "添加纪实")
		appendMenuItem(menu, mfString, cmEdit, "编辑档案")
		label := "标记为已解决"
		if pr.Resolved {
			label = "标记为未解决"
		}
		appendMenuItem(menu, mfString, cmResolve, label)
	} else if it := app.findItem(id); it != nil {
		appendMenuItem(menu, mfString, cmEdit, "编辑")
		if it.Type == TypeNote {
			label := "置顶"
			if it.Pinned {
				label = "取消置顶"
			}
			appendMenuItem(menu, mfString, cmPin, label)
		}
	} else {
		syscall.SyscallN(procDestroyMenu.Addr(), menu)
		return
	}
	syscall.SyscallN(procAppendMenu.Addr(), menu, mfSeparator, 0, 0)
	appendMenuItem(menu, mfString, cmDelete, "删除")

	var pt struct{ X, Y int32 }
	syscall.SyscallN(procGetCursorPos.Addr(), uintptr(unsafe.Pointer(&pt)))
	forceForegroundWindow(hwnd, 300) // 菜单需要窗口在前台，否则一出现就被关掉
	cmd, _, _ := syscall.SyscallN(procTrackPopupMenu.Addr(),
		menu, tpmRightClick|tpmReturnCmd,
		uintptr(uint32(pt.X)), uintptr(uint32(pt.Y)), 0, hwnd, 0)
	syscall.SyscallN(procDestroyMenu.Addr(), menu)

	switch cmd {
	case cmEdit:
		hideAllPanels()
		if pr := app.findProb(id); pr != nil {
			openProbDialog(id)
		} else if it := app.findItem(id); it != nil {
			if it.Type == TypeTodo {
				openTodoDialog(id)
			} else {
				openNoteDialog(id)
			}
		}
	case cmAddLog:
		openLogDialog(id, "")
	case cmResolve:
		toggleProbResolved(id)
	case cmPin:
		togglePin(id)
	case cmDelete:
		if !confirmBox(hwnd, "删除确认", "确定要删除这条记录吗？") {
			return
		}
		doDelete(id)
	}
}

// doDelete 删除记录或问题（右键菜单确认后调用）
func doDelete(id string) {
	if pr := app.findProb(id); pr != nil {
		deleteProblemFile(pr.ID)
		delete(app.probExpanded, id)
		rescanProjects()
		renderList()
		return
	}
	if pid, i := app.findItemPos(id); i >= 0 {
		app.items[pid] = append(app.items[pid][:i], app.items[pid][i+1:]...)
		saveProject(pid)
		renderList()
	}
}

// ---------- 对话框通用 ----------

func showDialog(id string) {
	el := app.gw.GetElementById(id)
	if el != nil {
		el.Show()
	}
	// 应用用户上次拖拉记住的尺寸（如 "dlg-note-mask" → "note"）
	kind := strings.TrimSuffix(strings.TrimPrefix(id, "dlg-"), "-mask")
	if kind != id && kind != "" {
		applyDlgSize(kind)
	}
	// 对话框居中（HTMLayout 对 fixed + left:50% + 负 margin 的居中支持不完善，
	// 改为显式计算 left/top，避免偏右/偏下）
	centerDialog(id)
}

// centerDialog 把对话框在窗口客户区内显式居中（替代 CSS left:50% + 负 margin）
func centerDialog(id string) {
	// 对话框本体 id = mask id 去掉 -mask 后缀
	dlgID := strings.TrimSuffix(id, "-mask")
	el := app.gw.GetElementById(dlgID)
	if el == nil {
		return
	}
	root := gohl.RootElement(app.gw.GetHwnd())
	if root == nil {
		return
	}
	// 窗口客户区宽高
	cw, ch := getClientSize(uintptr(app.gw.GetHwnd()))
	if cw <= 0 || ch <= 0 {
		return
	}
	// 对话框自身尺寸：必须用 BorderBoxSize（含 padding+border）——
	// HTMLayout 不支持 box-sizing，CSS 里的 width/height 只是内容盒，
	// 用 ContentBoxSize 居中会把对话框整体偏左上约半个 padding
	dw, dh := el.BorderBoxSize()
	if dw <= 0 || dh <= 0 {
		return
	}
	left := (cw - dw) / 2
	top := (ch - dh) / 2
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	el.SetStyle("left", fmt.Sprint(left)+"px")
	el.SetStyle("top", fmt.Sprint(top)+"px")
	// 清掉 CSS 的 margin-left/margin-top（否则会再偏移一次）
	el.SetStyle("margin-left", "0px")
	el.SetStyle("margin-top", "0px")
	el.Update(true, true, true, true, true)
}

// recenterVisibleDialog 窗口尺寸变化后把当前打开的对话框重新居中
// （否则拉伸窗口时对话框停在原位，会偏出可见区域）
func recenterVisibleDialog() {
	for _, kind := range []string{"note", "todo", "prob", "log", "proj"} {
		mask := app.gw.GetElementById("dlg-" + kind + "-mask")
		if mask != nil && panelVisible(mask) {
			centerDialog("dlg-" + kind + "-mask")
			return
		}
	}
}

// ---------- 子界面拖拉缩放（右下角把手） ----------

// dlgMin/Max 每个子界面内容区的最小/最大尺寸（含 padding 前的内容盒）
var dlgLimits = map[string][4]int{
	// kind: {minW, minH, maxW, maxH}
	"note": {360, 320, 820, 900},
	"todo": {360, 380, 820, 980},
	"prob": {360, 300, 820, 900},
	"log":  {360, 220, 820, 800},
	"proj": {360, 300, 860, 960},
}

// startDlgDrag 按下拖拉把手：记录起点，进入定时器拖拉循环
func startDlgDrag(kind string) {
	el := app.gw.GetElementById("dlg-" + kind)
	if el == nil {
		return
	}
	// ContentBoxSize = CSS width/height（HTMLayout 无 box-sizing，style 即内容盒）
	w, h := el.ContentBoxSize()
	x, y := getCursorPos()
	app.dlgDrag = &dlgDragState{kind: kind, startX: x, startY: y, startW: w, startH: h}
	app.gw.SetTimer(30, dlgDragTick)
}

// dlgDragTick 拖拉循环：跟随光标改对话框宽高；松手结束并记住尺寸
func dlgDragTick() {
	d := app.dlgDrag
	if d == nil {
		return
	}
	if !isLButtonDown() {
		// 松手：记住本次内容盒尺寸
		app.dlgDrag = nil
		el := app.gw.GetElementById("dlg-" + d.kind)
		if el != nil {
			if app.data.DlgSizes == nil {
				app.data.DlgSizes = map[string][2]int{}
			}
			w, h := el.ContentBoxSize()
			app.data.DlgSizes[d.kind] = [2]int{w, h}
			save()
		}
		return
	}
	cx, cy := getCursorPos()
	nw := d.startW + (cx - d.startX)
	nh := d.startH + (cy - d.startY)
	lim := dlgLimits[d.kind]
	if lim[0] > 0 {
		if nw < lim[0] {
			nw = lim[0]
		}
		if nh < lim[1] {
			nh = lim[1]
		}
		if nw > lim[2] {
			nw = lim[2]
		}
		if nh > lim[3] {
			nh = lim[3]
		}
	}
	// 不允许超出窗口客户区（外框约多 34px = padding 32 + border 2）
	if cw, ch := getClientSize(uintptr(app.gw.GetHwnd())); cw > 0 {
		if nw > cw-36 {
			nw = cw - 36
		}
		if nh > ch-36 {
			nh = ch - 36
		}
	}
	el := app.gw.GetElementById("dlg-" + d.kind)
	if el == nil {
		app.dlgDrag = nil
		return
	}
	el.SetStyle("width", fmt.Sprint(nw)+"px")
	el.SetStyle("height", fmt.Sprint(nh)+"px")
	el.Update(true, true, true, true, true)
	app.gw.SetTimer(30, dlgDragTick)
}

// applyDlgSize 打开子界面时应用用户上次拖拉记住的尺寸（没有则用 CSS 默认）
func applyDlgSize(kind string) {
	sz, ok := app.data.DlgSizes[kind]
	if !ok || sz[0] <= 0 || sz[1] <= 0 {
		return
	}
	el := app.gw.GetElementById("dlg-" + kind)
	if el == nil {
		return
	}
	lim := dlgLimits[kind]
	w, h := sz[0], sz[1]
	if lim[0] > 0 {
		if w < lim[0] {
			w = lim[0]
		}
		if h < lim[1] {
			h = lim[1]
		}
		if w > lim[2] {
			w = lim[2]
		}
		if h > lim[3] {
			h = lim[3]
		}
	}
	el.SetStyle("width", fmt.Sprint(w)+"px")
	el.SetStyle("height", fmt.Sprint(h)+"px")
}

func hideDialog(id string) {
	el := app.gw.GetElementById(id)
	if el != nil {
		el.Hide()
	}
}

// ---------- 记事对话框 ----------

// presetProjectsFromFilter 顶部筛选到具体项目时，新建记事/待办/问题默认带上该项目
// （筛选"全部/未指定/分类"时不预选；只对新增生效，编辑保持原项目不变）
func presetProjectsFromFilter() []string {
	fid := app.data.FilterProjectID
	if fid == "" || fid == FilterAll {
		return nil
	}
	if _, isCat := parseCatFilter(fid); isCat {
		return nil
	}
	if _, _, ok := app.data.findProjectFull(fid); ok {
		return []string{fid}
	}
	return nil
}

// presetCatFromFilter 预选项目后把分类同步为该项目的分类（候选面板按分类过滤）
func presetCatFromFilter(kind string) {
	ids := projIDsOf(kind)
	if len(ids) == 0 {
		return
	}
	cid := app.data.projectCategoryID(ids[0])
	switch kind {
	case "note":
		app.noteCatID = cid
	case "todo":
		app.todoCatID = cid
	case "prob":
		app.probCatID = cid
	}
}

func openNoteDialog(id string) {
	app.editing = id
	app.draftLinks = nil
	app.noteCatID = ""
	app.noteProjIDs = nil
	rescanProjects() // 分类/项目以磁盘文件为准
	if id == "" {
		app.noteProjIDs = presetProjectsFromFilter()
		presetCatFromFilter("note")
	}

	now := time.Now()
	noteDate, noteTime := now.Format("2006-01-02"), now.Format("15:04:05")
	setProgValue("note-text", "")
	setProgValue("note-proj-input", "")

	// 默认不置顶
	if el := app.gw.GetElementById("note-pin-on"); el != nil {
		el.RemoveClass("on")
	}
	if el := app.gw.GetElementById("note-pin-off"); el != nil {
		el.AddClass("on")
	}

	if id != "" {
		if it := app.findItem(id); it != nil {
			setProgValue("note-text", it.Text)
			app.noteCatID = app.data.projectCategoryID(it.ProjectID)
			app.noteProjIDs = itemProjects(*it)
			noteDate, noteTime = it.Time.Format("2006-01-02"), it.Time.Format("15:04:05")
			for _, l := range it.Links {
				app.draftLinks = append(app.draftLinks, l.Path)
			}
			if it.Pinned {
				if el := app.gw.GetElementById("note-pin-on"); el != nil {
					el.AddClass("on")
				}
				if el := app.gw.GetElementById("note-pin-off"); el != nil {
					el.RemoveClass("on")
				}
			}
		}
	}
	refreshDateTimeInputs("note-time-row", "note-date", "note-time", noteDate, noteTime, "时间")
	app.gw.GetElementById("note-cat-label").SetText(catDisplayName(app.noteCatID))
	renderDraftLinks()
	renderProjSelection("note")
	// 下拉默认收起：项目面板仅在输入项目号（或按 ▼）时才展开
	hideDialog("note-cat-panel")
	hideDialog("note-proj-panel")
	showDialog("dlg-note-mask")
}

// refreshDateTimeInputs 重建 date/time 输入行（带 value 属性）。
// gohl 的 SetValue 走 SET_TEXT_VALUE，date/time 行为不处理该方法，
// 唯一可靠做法是重建元素让 value 属性随元素初始化生效。
func refreshDateTimeInputs(rowID string, dateID, timeID, dateVal, timeVal, label string) {
	row := app.gw.GetElementById(rowID)
	if row == nil {
		return
	}
	row.SetHtml(`<div class="dlg-label">` + label + `</div>` +
		`<input id="` + dateID + `" type="date" class="dlg-input" style="width:150px;" value="` + dateVal + `"></input>` +
		`<input id="` + timeID + `" type="time" class="dlg-input" style="width:110px;margin-left:8px;" value="` + timeVal + `"></input>`)
	setInputPreset(dateID, dateVal)
	setInputPreset(timeID, timeVal)
}

// refreshDueInputs 重建待办完成日期行（日期 + 时刻）
func refreshDueInputs(dateVal, timeVal string) {
	row := app.gw.GetElementById("todo-due-row")
	if row == nil {
		return
	}
	row.SetHtml(`<div class="dlg-label">完成日期</div>` +
		`<input id="todo-due" type="date" class="dlg-input" style="width:150px;" value="` + dateVal + `"></input>` +
		`<input id="todo-due-time" type="time" class="dlg-input" style="width:100px;margin-left:8px;" value="` + timeVal + `"></input>` +
		`<div style="width:1*;font-size:9pt;color:#9ab5a8;padding-left:8px;">默认 17:00</div>`)
	setInputPreset("todo-due", dateVal)
	setInputPreset("todo-due-time", timeVal)
}

func renderDraftLinks() {
	el := app.gw.GetElementById("note-links")
	if el == nil {
		return
	}
	if len(app.draftLinks) == 0 {
		el.SetHtml(`<div class="links-empty">暂未添加链接</div>`)
		return
	}
	var sb strings.Builder
	for _, p := range app.draftLinks {
		name := baseName(p)
		sb.WriteString(`<div class="link-edit-row" data-path="` + html.EscapeString(p) + `"><span class="link-edit-path">` + html.EscapeString(name) +
			`</span><div class="link-edit-del" data-act="link-del">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

func addDraftLink(path string) {
	app.draftLinks = append(app.draftLinks, path)
	renderDraftLinks()
}

// renderTodoDraftLinks 待办对话框的暂存链接列表
func renderTodoDraftLinks() {
	el := app.gw.GetElementById("todo-links")
	if el == nil {
		return
	}
	if len(app.todoDraftLinks) == 0 {
		el.SetHtml(`<div class="links-empty">暂未添加链接</div>`)
		return
	}
	var sb strings.Builder
	for _, p := range app.todoDraftLinks {
		name := baseName(p)
		sb.WriteString(`<div class="link-edit-row" data-path="` + html.EscapeString(p) + `"><span class="link-edit-path">` + html.EscapeString(name) +
			`</span><div class="link-edit-del" data-act="link-del">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

// currentLinkTarget 判断当前哪个对话框在最前（选文件按钮共用）
func currentLinkTarget() string {
	if el := app.gw.GetElementById("dlg-log-mask"); el != nil && panelVisible(el) {
		return "log"
	}
	if el := app.gw.GetElementById("dlg-prob-mask"); el != nil && panelVisible(el) {
		return "prob"
	}
	if el := app.gw.GetElementById("dlg-todo-mask"); el != nil && panelVisible(el) {
		return "todo"
	}
	return "note"
}

// addDraftLinkTo 向指定对话框的暂存链接列表添加路径
func addDraftLinkTo(target, path string) {
	switch target {
	case "prob":
		app.probDraftLinks = append(app.probDraftLinks, path)
		renderProbDraftLinks()
	case "log":
		app.logDraftLinks = append(app.logDraftLinks, path)
		renderLogDraftLinks()
	case "todo":
		app.todoDraftLinks = append(app.todoDraftLinks, path)
		renderTodoDraftLinks()
	default:
		addDraftLink(path)
	}
}

// removeDraftLinkFrom 从指定对话框的暂存链接列表移除一行
func removeDraftLinkFrom(target string, elem *gohl.Element) {
	row := elem.SelectParent(".link-edit-row")
	if row == nil {
		return
	}
	p, ok := row.Attr("data-path")
	if !ok {
		return
	}
	removeStr := func(list []string) []string {
		out := list[:0]
		for _, dp := range list {
			if dp != p {
				out = append(out, dp)
			}
		}
		return out
	}
	switch target {
	case "prob":
		app.probDraftLinks = removeStr(app.probDraftLinks)
		row.Detach()
		if len(app.probDraftLinks) == 0 {
			renderProbDraftLinks()
		}
	case "log":
		app.logDraftLinks = removeStr(app.logDraftLinks)
		row.Detach()
		if len(app.logDraftLinks) == 0 {
			renderLogDraftLinks()
		}
	case "todo":
		app.todoDraftLinks = removeStr(app.todoDraftLinks)
		row.Detach()
		if len(app.todoDraftLinks) == 0 {
			renderTodoDraftLinks()
		}
	default:
		app.draftLinks = removeStr(app.draftLinks)
		row.Detach()
		if len(app.draftLinks) == 0 {
			renderDraftLinks()
		}
	}
}

func saveNoteDialog() {
	text := strings.TrimSpace(app.gw.GetElementValue("note-text"))
	t := parseDateTime(inputValue("note-date"), inputValue("note-time"))

	if text == "" && len(app.draftLinks) == 0 {
		hideDialog("dlg-note-mask")
		return
	}

	var links []Link
	for _, p := range app.draftLinks {
		links = append(links, Link{Name: baseName(p), Path: p})
	}

	pinned := false
	if el := app.gw.GetElementById("note-pin-on"); el != nil {
		if cls, ok := el.Attr("class"); ok {
			for _, c := range strings.Fields(cls) {
				if c == "on" {
					pinned = true
				}
			}
		}
	}

	ids := resolveProjectInput("note")
	if app.editing != "" {
		if oldPID, i := app.findItemPos(app.editing); i >= 0 {
			it := app.items[oldPID][i]
			it.Text = text
			it.Time = t
			it.Links = links
			it.Pinned = pinned
			setItemProjects(&it, ids)
			if oldPID == it.ProjectID {
				app.items[oldPID][i] = it
			} else {
				// 换了主项目：从旧项目移除、写入新项目，两个文件都落盘
				app.items[oldPID] = append(app.items[oldPID][:i], app.items[oldPID][i+1:]...)
				app.items[it.ProjectID] = append(app.items[it.ProjectID], it)
				saveProject(oldPID)
			}
			saveProject(it.ProjectID)
		}
	} else {
		it := Item{
			ID:        newID(),
			Type:      TypeNote,
			Text:      text,
			Links:     links,
			Time:      t,
			Pinned:    pinned,
		}
		setItemProjects(&it, ids)
		app.items[it.ProjectID] = append(app.items[it.ProjectID], it)
		saveProject(it.ProjectID)
	}
	hideDialog("dlg-note-mask")
	renderList()
}

// ---------- 待办对话框 ----------

// openTodoDialog 新增/编辑待办。默认完成日期 = 当天 17:00。支持子待办和附件链接。
func openTodoDialog(id string) {
	app.editing = id
	app.todoCatID = ""
	app.todoProjIDs = nil
	app.draftSubs = nil
	app.todoDraftLinks = nil
	rescanProjects() // 分类/项目以磁盘文件为准
	if id == "" {
		app.todoProjIDs = presetProjectsFromFilter()
		presetCatFromFilter("todo")
	}

	now := time.Now()
	tdDate, tdTime := now.Format("2006-01-02"), now.Format("15:04:05")
	dueDate, dueTime := now.Format("2006-01-02"), "17:00:00"
	setProgValue("todo-text", "")
	setProgValue("todo-proj-input", "")
	setProgValue("todo-sub-input", "")

	// 默认「普通」
	setTodoPriority("normal")

	if id != "" {
		if it := app.findItem(id); it != nil {
			setProgValue("todo-text", it.Text)
			app.todoCatID = app.data.projectCategoryID(it.ProjectID)
			app.todoProjIDs = itemProjects(*it)
			if it.DueDate != "" {
				dueDate = it.DueDate
			}
			dueTime = it.DueTime
			if dueTime == "" {
				dueTime = "17:00:00"
			} else if len(dueTime) == 5 {
				dueTime += ":00"
			}
			tdDate, tdTime = it.Time.Format("2006-01-02"), it.Time.Format("15:04:05")
			setTodoPriority(it.Priority())
			for _, st := range it.SubTodos {
				app.draftSubs = append(app.draftSubs, st)
			}
			for _, l := range it.Links {
				app.todoDraftLinks = append(app.todoDraftLinks, l.Path)
			}
		}
	}
	refreshDueInputs(dueDate, dueTime)
	refreshDateTimeInputs("todo-time-row", "todo-date", "todo-time", tdDate, tdTime, "时间")
	app.gw.GetElementById("todo-cat-label").SetText(catDisplayName(app.todoCatID))
	renderDraftSubs()
	renderTodoDraftLinks()
	renderProjSelection("todo")
	// 下拉默认收起：项目面板仅在输入项目号（或按 ▼）时才展开
	hideDialog("todo-cat-panel")
	hideDialog("todo-proj-panel")
	showDialog("dlg-todo-mask")
}

// setTodoPriority 待办优先级三选一："urgent" 紧急 | "normal" 普通 | "low" 非紧急
func setTodoPriority(p string) {
	on := map[string]bool{"urgent": false, "normal": false, "low": false}
	if _, ok := on[p]; ok {
		on[p] = true
	} else {
		on["normal"] = true
	}
	for id, active := range on {
		el := app.gw.GetElementById("todo-" + id)
		if el == nil {
			continue
		}
		if active {
			el.AddClass("on")
		} else {
			el.RemoveClass("on")
		}
	}
}

// pickedPriority 读取对话框当前选中的优先级
func pickedPriority() string {
	for _, id := range []string{"urgent", "normal", "low"} {
		el := app.gw.GetElementById("todo-" + id)
		if el == nil {
			continue
		}
		if cls, ok := el.Attr("class"); ok {
			for _, c := range strings.Fields(cls) {
				if c == "on" {
					return id
				}
			}
		}
	}
	return "normal"
}

func saveTodoDialog() {
	text := strings.TrimSpace(app.gw.GetElementValue("todo-text"))
	if text == "" && len(app.todoDraftLinks) == 0 {
		hideDialog("dlg-todo-mask")
		return
	}
	due := strings.TrimSpace(inputValue("todo-due"))
	dueTime := strings.TrimSpace(inputValue("todo-due-time"))
	if due != "" && dueTime == "" {
		dueTime = "17:00"
	}
	if due == "" {
		dueTime = ""
	}
	t := parseDateTime(inputValue("todo-date"), inputValue("todo-time"))

	prio := pickedPriority()
	urgent := prio == "urgent"
	low := prio == "low"

	var links []Link
	for _, p := range app.todoDraftLinks {
		links = append(links, Link{Name: baseName(p), Path: p})
	}

	ids := resolveProjectInput("todo")
	if app.editing != "" {
		if oldPID, i := app.findItemPos(app.editing); i >= 0 {
			it := app.items[oldPID][i]
			it.Text = text
			it.Time = t
			it.DueDate = due
			it.DueTime = dueTime
			it.Urgent = urgent
			it.Low = low
			it.Links = links
			it.SubTodos = app.draftSubs
			setItemProjects(&it, ids)
			if oldPID == it.ProjectID {
				app.items[oldPID][i] = it
			} else {
				app.items[oldPID] = append(app.items[oldPID][:i], app.items[oldPID][i+1:]...)
				app.items[it.ProjectID] = append(app.items[it.ProjectID], it)
				saveProject(oldPID)
			}
			saveProject(it.ProjectID)
		}
	} else {
		it := Item{
			ID:        newID(),
			Type:      TypeTodo,
			Text:      text,
			Time:      t,
			Urgent:    urgent,
			Low:       low,
			DueDate:   due,
			DueTime:   dueTime,
			SubTodos:  app.draftSubs,
			Links:     links,
		}
		setItemProjects(&it, ids)
		app.items[it.ProjectID] = append(app.items[it.ProjectID], it)
		saveProject(it.ProjectID)
	}
	hideDialog("dlg-todo-mask")
	renderList()
}

// ---------- 项目管理对话框 ----------

func openProjDialog() {
	rescanProjects() // 分类/项目以磁盘文件为准，打开时刷新
	renderProjDialog()
	renderFilterPanel()
	renderList()
	showDialog("dlg-proj-mask")
}

// renderProjDialog 两级结构：分类标题 → 项目网格（每行固定 4 个）→ 行内添加框。
// 这里只做展示与新建；删除请直接删程序目录下对应的「分类-项目.json」文件。
// 点项目名右侧 ✎ 可编辑该项目别名（搜索/输入均可用别名命中）。
func renderProjDialog() {
	body := app.gw.GetElementById("proj-body")
	if body == nil {
		return
	}
	var sb strings.Builder
	idx := 0
	for _, c := range app.data.Categories {
		sb.WriteString(`<div class="proj-cat"><div class="proj-cat-head">` +
			`<div class="proj-cat-name">` + html.EscapeString(c.Name) + `</div>` +
			`<div class="proj-cat-count">` + fmt.Sprint(len(c.Projects)) + ` 个项目</div></div>`)
		if len(c.Projects) > 0 {
			// 每行固定 4 个：手工分组渲染，保证任何界面宽度下都是 4 列
			for row := 0; row < len(c.Projects); row += 4 {
				end := row + 4
				if end > len(c.Projects) {
					end = len(c.Projects)
				}
				sb.WriteString(`<div class="proj-row">`)
				for _, p := range c.Projects[row:end] {
					alias := strings.Join(app.data.aliasesOf(p.ID), "、")
					aliasHTML := ""
					if alias != "" {
						aliasHTML = `<span class="proj-item-alias">` + html.EscapeString(alias) + `</span>`
					}
					sb.WriteString(`<div class="proj-cell">` +
						`<span class="proj-item-name">` + html.EscapeString(p.Name) + `</span>` +
						aliasHTML +
						`<div class="proj-alias-btn" data-act="proj-alias-edit" data-id="` + html.EscapeString(p.ID) + `">✎</div></div>`)
				}
				// 行尾不足 4 个时补空位，保持网格对齐
				for i := end - row; i < 4; i++ {
					sb.WriteString(`<div class="proj-cell proj-cell-empty"></div>`)
				}
				sb.WriteString(`</div>`)
				// 该行里正在编辑别名的项目 → 行下插编辑器
				for _, p := range c.Projects[row:end] {
					if app.editingAliasPID == p.ID {
						sb.WriteString(renderAliasEditor(p))
					}
				}
			}
		}
		// 行内"在该分类下添加项目号"（添加即创建 分类-项目.json）
		inputID := fmt.Sprintf("proj-add-input-%d", idx)
		sb.WriteString(`<div class="proj-add-row"><span class="proj-add-label">新项目</span>` +
			`<input class="proj-add-input" id="` + inputID + `" type="text"></input>` +
			`<div class="btn-sm primary" data-act="proj-add-item" data-cat="` + html.EscapeString(c.Name) +
			`" data-input="` + inputID + `" style="width:56px;margin-left:8px;">添加</div></div>`)
		sb.WriteString(`</div>`)
		idx++
	}
	if idx == 0 {
		sb.WriteString(`<div class="proj-empty">还没有分类和项目<br/>在上方输入分类名 + 项目号创建</div>`)
	}
	body.SetHtml(sb.String())
	// 别名编辑器打开时把当前别名填进输入框（SetHtml 后元素是新的）
	if app.editingAliasPID != "" {
		if _, p, ok := app.data.findProjectFull(app.editingAliasPID); ok {
			setProgValue("proj-alias-input", strings.Join(app.data.aliasesOf(p.ID), "、"))
		}
	}
}

// renderAliasEditor 项目别名编辑行（项目下方展开）
func renderAliasEditor(p Project) string {
	return `<div class="proj-alias-edit">` +
		`<span class="proj-add-label">别名</span>` +
		`<input class="proj-add-input" id="proj-alias-input" type="text"></input>` +
		`<div class="btn-sm primary" data-act="proj-alias-save" data-id="` + html.EscapeString(p.ID) + `" style="width:56px;margin-left:8px;">保存</div>` +
		`<div class="btn-sm" data-act="proj-alias-cancel" style="width:56px;margin-left:6px;">取消</div>` +
		`<span class="proj-alias-tip">多个别名用 、 分隔；搜索/输入可用别名命中</span></div>`
}

// createProjectFile 界面上新建分类/项目 = 在程序目录创建「分类-项目.json」。
// 分类名里的 "-" 会替换为 "_"（文件名按第一个 "-" 拆分类与项目）。
func createProjectFile(catName, projName string) {
	id, ok := buildProjectID(catName, projName)
	if !ok {
		return
	}
	ensureProjectFile(id)
	rescanProjects()
	if el := app.gw.GetElementById("proj-new-cat"); el != nil {
		setProgValue("proj-new-cat", "")
	}
	if el := app.gw.GetElementById("proj-new-proj"); el != nil {
		setProgValue("proj-new-proj", "")
	}
	renderProjDialog()
	renderFilterPanel()
	renderList()
}

// ---------- 问题排查对话框 ----------

// openProbDialog 新增/编辑问题。档案文本 + 档案链接 + 分类/项目 + 时间。
func openProbDialog(id string) {
	app.editingProb = id
	app.probCatID = ""
	app.probProjIDs = nil
	app.probDraftLinks = nil
	rescanProjects() // 分类/项目以磁盘文件为准
	if id == "" {
		app.probProjIDs = presetProjectsFromFilter()
		presetCatFromFilter("prob")
	}

	now := time.Now()
	pDate, pTime := now.Format("2006-01-02"), now.Format("15:04:05")
	setProgValue("prob-name", "")
	setProgValue("prob-profile", "")
	setProgValue("prob-proj-input", "")

	if id != "" {
		if pr := app.findProb(id); pr != nil {
			setProgValue("prob-name", pr.Name)
			setProgValue("prob-profile", pr.ProfileText)
			app.probCatID = app.data.projectCategoryID(pr.ProjectID)
			app.probProjIDs = probProjects(*pr)
			pDate, pTime = pr.Time.Format("2006-01-02"), pr.Time.Format("15:04:05")
			for _, l := range pr.ProfileLinks {
				app.probDraftLinks = append(app.probDraftLinks, l.Path)
			}
		}
	}
	refreshDateTimeInputs("prob-time-row", "prob-date", "prob-time", pDate, pTime, "时间")
	app.gw.GetElementById("prob-cat-label").SetText(catDisplayName(app.probCatID))
	renderProbDraftLinks()
	renderProjSelection("prob")
	// 下拉默认收起：项目面板仅在输入项目号（或按 ▼）时才展开
	hideDialog("prob-cat-panel")
	hideDialog("prob-proj-panel")
	showDialog("dlg-prob-mask")
}

func renderProbDraftLinks() {
	el := app.gw.GetElementById("prob-links")
	if el == nil {
		return
	}
	if len(app.probDraftLinks) == 0 {
		el.SetHtml(`<div class="links-empty">暂未添加链接</div>`)
		return
	}
	var sb strings.Builder
	for _, p := range app.probDraftLinks {
		name := baseName(p)
		sb.WriteString(`<div class="link-edit-row" data-path="` + html.EscapeString(p) + `"><span class="link-edit-path">` + html.EscapeString(name) +
			`</span><div class="link-edit-del" data-act="prob-link-del">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

// saveProbDialog 保存问题：新文件名 = 分类-项目-问题名称.json（改名/换项目 → 重建文件）
func saveProbDialog() {
	name := strings.TrimSpace(app.gw.GetElementValue("prob-name"))
	if name == "" {
		hideDialog("dlg-prob-mask")
		return
	}
	profile := strings.TrimSpace(app.gw.GetElementValue("prob-profile"))
	t := parseDateTime(inputValue("prob-date"), inputValue("prob-time"))
	ids := resolveProjectInput("prob")

	var links []Link
	for _, p := range app.probDraftLinks {
		links = append(links, Link{Name: baseName(p), Path: p})
	}

	if app.editingProb != "" {
		if pr := app.findProb(app.editingProb); pr != nil {
			oldID := pr.ID
			pr.Name = name
			pr.Time = t
			pr.ProfileText = profile
			pr.ProfileLinks = links
			setProbProjects(pr, ids)
			newID, ok := buildProblemID(pr.ProjectID, name)
			if !ok {
				hideDialog("dlg-prob-mask")
				return
			}
			if newID != oldID {
				// 文件名 = 身份：改名/换项目要重建文件
				pr.ID = newID
				if err := saveProblem(pr); err != nil {
					fmt.Println("保存问题失败:", err)
				}
				deleteProblemFile(oldID)
			} else {
				saveProblem(pr)
			}
		}
	} else {
		primary := ""
		if len(ids) > 0 {
			primary = ids[0]
		}
		newID, ok := buildProblemID(primary, name)
		if !ok {
			hideDialog("dlg-prob-mask")
			return
		}
		pr := Problem{
			ID: newID, Name: name, Time: t,
			ProfileText: profile, ProfileLinks: links,
		}
		setProbProjects(&pr, ids)
		if err := saveProblem(&pr); err != nil {
			fmt.Println("保存问题失败:", err)
		}
		app.probs = append(app.probs, pr)
	}
	hideDialog("dlg-prob-mask")
	app.editingProb = ""
	rescanProjects()
	renderList()
}

// ---------- 纪实条目对话框 ----------

// openLogDialog 给问题添加/编辑一条纪实
func openLogDialog(probID, entryID string) {
	app.logProbID = probID
	app.logEntryID = entryID
	app.logDraftLinks = nil

	now := time.Now()
	eDate, eTime := now.Format("2006-01-02"), now.Format("15:04:05")
	setProgValue("log-text", "")

	if entryID != "" {
		if pr := app.findProb(probID); pr != nil {
			for _, e := range pr.Log {
				if e.ID == entryID {
					setProgValue("log-text", e.Text)
					eDate, eTime = e.Time.Format("2006-01-02"), e.Time.Format("15:04:05")
					for _, l := range e.Links {
						app.logDraftLinks = append(app.logDraftLinks, l.Path)
					}
					break
				}
			}
		}
	}
	refreshDateTimeInputs("log-time-row", "log-date", "log-time", eDate, eTime, "时间")
	title := "添加纪实"
	if entryID != "" {
		title = "编辑纪实"
	}
	if el := app.gw.GetElementById("log-title"); el != nil {
		el.SetText(title)
	}
	renderLogDraftLinks()
	showDialog("dlg-log-mask")
}

func renderLogDraftLinks() {
	el := app.gw.GetElementById("log-links")
	if el == nil {
		return
	}
	if len(app.logDraftLinks) == 0 {
		el.SetHtml(`<div class="links-empty">暂未添加链接</div>`)
		return
	}
	var sb strings.Builder
	for _, p := range app.logDraftLinks {
		name := baseName(p)
		sb.WriteString(`<div class="link-edit-row" data-path="` + html.EscapeString(p) + `"><span class="link-edit-path">` + html.EscapeString(name) +
			`</span><div class="link-edit-del" data-act="log-link-del">×</div></div>`)
	}
	el.SetHtml(sb.String())
}

// saveLogDialog 保存纪实条目到问题的 Log 列表
func saveLogDialog() {
	text := strings.TrimSpace(app.gw.GetElementValue("log-text"))
	if text == "" && len(app.logDraftLinks) == 0 {
		hideDialog("dlg-log-mask")
		return
	}
	t := parseDateTime(inputValue("log-date"), inputValue("log-time"))
	var links []Link
	for _, p := range app.logDraftLinks {
		links = append(links, Link{Name: baseName(p), Path: p})
	}
	pr := app.findProb(app.logProbID)
	if pr == nil {
		hideDialog("dlg-log-mask")
		return
	}
	if app.logEntryID != "" {
		for i := range pr.Log {
			if pr.Log[i].ID == app.logEntryID {
				pr.Log[i].Text = text
				pr.Log[i].Time = t
				pr.Log[i].Links = links
				break
			}
		}
	} else {
		pr.Log = append(pr.Log, ProblemEntry{ID: newID(), Text: text, Links: links, Time: t})
	}
	saveProblem(pr)
	hideDialog("dlg-log-mask")
	app.probExpanded[pr.ID] = true // 新纪实后自动展开便于查看
	renderList()
}

// ---------- 工具 ----------

// uiTestHook 开发自测：按环境变量 WORKLOG_UI_TEST 启动即打开指定界面，便于截图验收。
// 不设置时为空操作，正常使用不受影响。
func uiTestHook() {
	switch strings.ToLower(os.Getenv("WORKLOG_UI_TEST")) {
	case "filter":
		onClick(app.gw.GetElementById("filter-btn"))
	case "date":
		onClick(app.gw.GetElementById("btn-date"))
	case "proj":
		openProjDialog()
	case "note":
		openNoteDialog("")
	case "todo":
		openTodoDialog("")
	case "noteproj":
		openNoteDialog("")
		renderProjOptions("note")
	case "projflow":
		// 验证项目号下拉交互：打开不展开 → 输入弹出 → 点选即确认收起 → 清空收起
		openNoteDialog("")
		debugLayout("projflow-open")
		setProgValue("note-proj-input", "1")
		onProjInputChanged("note")
		debugLayout("projflow-typed")
		if p := app.gw.GetElementById("note-proj-panel"); p != nil {
			var target *gohl.Element
			for _, it := range p.Select("[data-act=proj-pick]") {
				if v, ok := it.Attr("data-id"); ok && v != "" {
					target = it
					break
				}
			}
			if target != nil {
				onClick(target)
			}
		}
		debugLayout("projflow-picked")
		setProgValue("note-proj-input", "1")
		onProjInputChanged("note")
		setProgValue("note-proj-input", "")
		onProjInputChanged("note")
		debugLayout("projflow-cleared")
	case "todocat":
		openTodoDialog("")
		if p := app.gw.GetElementById("todo-cat-panel"); p != nil {
			p.SetHtml(buildCatPanelHTML(app.todoCatID))
			p.Show()
		}
	case "prob":
		openProbDialog("")
	case "problist":
		// 搜索定位到问题卡片并展开，便于截图验收
		app.filter.Keyword = "闪烁"
		setProgValue("search-input", "闪烁")
		for i := range app.probs {
			app.probExpanded[app.probs[i].ID] = true
		}
		renderList()
	case "log":
		if len(app.probs) > 0 {
			openLogDialog(app.probs[0].ID, "")
		}
	case "subtodo":
		openTodoDialog("")
		app.draftSubs = []SubTodo{{ID: newID(), Text: "子待办一"}, {ID: newID(), Text: "子待办二", Done: true}}
		renderDraftSubs()
	case "lowopen":
		// 展开非紧急待办分组（默认收纳，截图验收用）
		app.lowOpen = true
		renderList()
	case "lowbar":
		// 搜索定位到非紧急分组，验证收纳条（收起态）
		app.filter.Keyword = "归档"
		setProgValue("search-input", "归档")
		renderList()
	case "lowbaropen":
		app.lowOpen = true
		app.filter.Keyword = "归档"
		setProgValue("search-input", "归档")
		renderList()
	case "editnote":
		// 打开一条已有中文记事，验证回填正确（SetValue 中文长度 bug 回归）
		for _, its := range app.items {
			for _, it := range its {
				if it.Type == TypeNote && !it.Pinned {
					openNoteDialog(it.ID)
					return
				}
			}
		}
	case "edittodo":
		// 打开一条已有待办，验证三档优先级回填
		for _, its := range app.items {
			for _, it := range its {
				if it.Type == TypeTodo {
					openTodoDialog(it.ID)
					return
				}
			}
		}
	case "editprob":
		if len(app.probs) > 0 {
			openProbDialog(app.probs[0].ID)
		}
	}
}

func (a *appState) findItem(id string) *Item {
	if pid, i := a.findItemPos(id); i >= 0 {
		return &a.items[pid][i]
	}
	return nil
}

// inputValue 读取输入控件值：优先 value 属性（date/time 行为不支持
// GET_TEXT_VALUE，只能从属性读；文本/textarea 则走行为方法）。
func inputValue(id string) string {
	// date/time 输入优先取 EDIT_VALUE_CHANGED 当下捕获的值（HTMLayout 的 value 属性
	// 在事件后可能回退陈旧初值，保存时直接读 Attr 会拿到旧日期）
	if v, ok := app.dateVals[id]; ok {
		return v
	}
	el := app.gw.GetElementById(id)
	if el == nil {
		return ""
	}
	if v, ok := el.Attr("value"); ok && v != "" {
		return v
	}
	if v, err := el.ValueAsString(); err == nil {
		return v
	}
	return ""
}

// setInputPreset 程序化重建 date/time 输入后，把预置值写入捕获表，
// 保证 inputValue 读到的是当前预置值（而非上次捕获的旧值）。
func setInputPreset(id, v string) {
	if app.dateVals == nil {
		app.dateVals = map[string]string{}
	}
	if v == "" {
		delete(app.dateVals, id)
	} else {
		app.dateVals[id] = v
	}
}

// setProgValue 程序化回填输入框（抑制 EDIT_VALUE_CHANGED 误触发自动弹候选面板）
func setProgValue(id, v string) {
	app.suppressEdit = true
	app.gw.SetElementValue(id, v)
	app.suppressEdit = false
}

// parseDateTime 合并 date + time value 为 time.Time；失败回退当前时间
func parseDateTime(dateVal, timeVal string) time.Time {
	d := strings.TrimSpace(dateVal)
	t := strings.TrimSpace(timeVal)
	// HTMLayout date/time 空态可能返回 "--/--/----" 之类，统一视为空
	if d == "" || strings.Count(d, "-") != 2 || strings.Contains(d, "/") {
		d = time.Now().Format("2006-01-02")
	}
	if t == "" || strings.Count(t, ":") < 1 {
		t = time.Now().Format("15:04:05")
	}
	if len(t) == 5 {
		t += ":00"
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", d+" "+t, time.Local); err == nil {
		return parsed
	}
	return time.Now()
}

// baseName 取路径最后一段作为显示名
func baseName(p string) string {
	p = strings.TrimRight(p, "\\/")
	if i := strings.LastIndexAny(p, "\\/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// 防止未使用告警
var _ = sort.Strings
var _ = strconv.Itoa
