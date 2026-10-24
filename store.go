package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------- 数据模型 ----------
//
// 分类与项目不再登记在配置文件里，而是由 exe 同目录下的存储文件决定：
//
//	分类-项目.json  一个项目的记录文件（文件名即身份；删除文件 = 删除该项目）
//	未指定.json     未指定项目的记录
//	工作记录.json   界面设置（置顶、筛选记忆）
//
// 每次启动 / 打开面板时扫描目录，重建分类树。界面上新建分类或项目，
// 就是创建对应的「分类-项目.json」文件。

// Category 项目分类（由文件名前缀归并而来）
type Category struct {
	ID       string    `json:"id"` // = 分类名
	Name     string    `json:"name"`
	Projects []Project `json:"projects"`
}

// Project 项目号（= 一个存储文件）
type Project struct {
	ID   string `json:"id"`   // = 文件基名「分类-项目」
	Name string `json:"name"` // = 项目名（文件名第一个 - 之后）
}

// Link 记事附带的文件/文件夹链接
type Link struct {
	Name string `json:"name"` // 显示名
	Path string `json:"path"` // 本地路径
}

// SubTodo 待办的子待办（简单清单项）
type SubTodo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done,omitempty"`
}

// Item 一条记录（记事或待办）
type Item struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "note" 记事 | "todo" 待办
	Text      string    `json:"text"`
	Links     []Link    `json:"links,omitempty"`
	ProjectID string    `json:"projectId"`            // 主项目 = 文件基名「分类-项目」，"" = 未指定
	ProjectIDs []string `json:"projectIds,omitempty"` // 多项目标记（为空时按 projectId 单项目）
	Time      time.Time `json:"time"`      // 创建时间（默认当前，可编辑）

	// 记事专用
	Pinned bool `json:"pinned,omitempty"` // 置顶记事，显示在列表最上面

	// 待办专用：三档优先级——紧急（Urgent）/ 普通（两者皆 false）/ 非紧急（Low）
	Urgent   bool      `json:"urgent,omitempty"`
	Low      bool      `json:"low,omitempty"` // 非紧急：排在普通待办之后，列表默认收纳
	DueDate  string    `json:"dueDate,omitempty"` // 完成日期 YYYY-MM-DD
	DueTime  string    `json:"dueTime,omitempty"` // 完成时刻 HH:MM（默认 17:00）
	Done     bool      `json:"done,omitempty"`
	DoneTime time.Time `json:"doneTime,omitempty"`
	SubTodos []SubTodo `json:"subTodos,omitempty"` // 子待办清单
}

// Priority 待办优先级：urgent 紧急 | normal 普通 | low 非紧急
func (it Item) Priority() string {
	switch {
	case it.Urgent:
		return "urgent"
	case it.Low:
		return "low"
	default:
		return "normal"
	}
}

// ProblemEntry 问题排查的一条纪实（可挂本地文件/文件夹链接）
type ProblemEntry struct {
	ID    string    `json:"id"`
	Text  string    `json:"text"`
	Links []Link    `json:"links,omitempty"`
	Time  time.Time `json:"time"`
}

// Problem 问题排查记录（独立存储：问题排查/分类-项目-问题名称.json）
type Problem struct {
	ID           string         `json:"id"`           // = 文件基名「分类-项目-问题名称」
	Name         string         `json:"name"`         // 问题名称
	ProjectID    string         `json:"projectId"`    // 主项目 = 项目文件基名「分类-项目」，"" = 未指定
	ProjectIDs   []string       `json:"projectIds,omitempty"` // 多项目标记（为空时按 projectId）
	Time         time.Time      `json:"time"`         // 发生/记录时间（默认当前，可编辑）
	Resolved     bool           `json:"resolved,omitempty"`
	ResolvedTime *time.Time     `json:"resolvedTime,omitempty"`
	ProfileText  string         `json:"profileText,omitempty"`  // 档案（问题描述/资料）
	ProfileLinks []Link         `json:"profileLinks,omitempty"` // 档案挂的链接
	Log          []ProblemEntry `json:"log,omitempty"`          // 纪实（处理过程，按时间追加）
}

// StoreData 主配置（exe 同目录 工作记录.json）：只剩界面设置。
// Categories 是扫描目录得到的运行时注册表，不落盘。
type StoreData struct {
	Version         int        `json:"version"`
	Topmost         bool       `json:"topmost"`         // 窗口置顶开关（持久化）
	FilterProjectID string     `json:"filterProjectId"` // "all" / 项目 ID / "cat:分类名"
	ProjectAliases  map[string][]string `json:"projectAliases,omitempty"` // 项目别名（项目ID -> 别名列表）
	DlgSizes        map[string][2]int   `json:"dlgSizes,omitempty"`       // 子界面记住的宽高（"note"->[w,h]）
	Categories      []Category `json:"-"`
}

// ProjectStore 单个项目的记录文件内容（明文 JSON）
type ProjectStore struct {
	Version int    `json:"version"`
	Project string `json:"project"` // 项目名
	Items   []Item `json:"items"`
}

const (
	TypeNote = "note"
	TypeTodo = "todo"

	FilterAll = "all"

	// 分类筛选值形如 "cat:分类名"，与项目 ID 共用 FilterProjectID 字段
	catFilterPrefix = "cat:"

	masterFileName = "工作记录.json"

	// 未指定项目的记录文件（文件名不含 "-"，不会被当成项目）
	noneProjectName    = "未指定"
	noneFileName       = "未指定.json"
	legacyNoneFileName = "记录_未指定.json"

	// 问题排查独立存储目录（文件名 = 分类-项目-问题名称.json）
	problemDirName = "问题排查"
)

// catFilterValue 生成分类筛选值
func catFilterValue(cid string) string { return catFilterPrefix + cid }

// parseCatFilter 解析分类筛选值；非分类筛选返回 false
func parseCatFilter(v string) (cid string, ok bool) {
	if strings.HasPrefix(v, catFilterPrefix) {
		return strings.TrimPrefix(v, catFilterPrefix), true
	}
	return "", false
}

func newID() string {
	return fmt.Sprintf("%d%06d", time.Now().UnixNano()/1e6, rand.Intn(1000000))
}

// ---------- 路径与文件名 ----------

// exeDirFn 便于单元测试替换 exe 目录（默认 os.Executable）
var exeDirFn = os.Executable

func exeDir() string {
	exe, err := exeDirFn()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// masterPath 主配置：exe 同级目录，便携拷走不丢数据
func masterPath() string {
	return filepath.Join(exeDir(), masterFileName)
}

// sanitizeFileName 去掉 Windows 文件名非法字符
func sanitizeFileName(name string) string {
	r := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return strings.TrimSpace(r.Replace(name))
}

// buildProjectID 由分类名 + 项目名生成项目 ID（= 文件基名「分类-项目」）。
// 分类名里的 "-" 替换为 "_"，保证能按第一个 "-" 反解回分类/项目。
func buildProjectID(catName, projName string) (string, bool) {
	cat := sanitizeFileName(strings.ReplaceAll(catName, "-", "_"))
	proj := sanitizeFileName(projName)
	if cat == "" || proj == "" {
		return "", false
	}
	return cat + "-" + proj, true
}

// parseProjectID 按第一个 "-" 拆出分类名与项目名
func parseProjectID(id string) (catName, projName string) {
	if i := strings.Index(id, "-"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}

// projectFileName 项目 ID → 文件名（"" = 未指定）
func projectFileName(id string) string {
	if id == "" {
		return noneFileName
	}
	return id + ".json"
}

func projectFilePath(id string) string {
	return filepath.Join(exeDir(), projectFileName(id))
}

// ---------- 问题排查存储（独立目录） ----------
//
//	问题排查/分类-项目-问题名称.json
//
// 文件名三段：项目 ID（分类-项目）+ "-" + 问题名称。反解时按已知项目 ID
// 最长前缀匹配（项目名允许含 "-"）；未指定项目用 "未指定-" 前缀。

func problemDir() string {
	return filepath.Join(exeDir(), problemDirName)
}

func problemFilePath(id string) string {
	return filepath.Join(problemDir(), id+".json")
}

// buildProblemID 由项目 ID + 问题名生成问题 ID（= 文件基名「分类-项目-问题名称」）
func buildProblemID(projID, probName string) (string, bool) {
	name := sanitizeFileName(strings.ReplaceAll(probName, "-", "_"))
	if name == "" {
		return "", false
	}
	if projID == "" {
		return noneProjectName + "-" + name, true
	}
	return projID + "-" + name, true
}

// parseProblemID 按已知项目 ID 反解问题文件基名（最长前缀匹配）。
// known 非空时优先精确匹配项目；否则走启发式（前两段为项目）。
func parseProblemID(base string, known []string) (projID, probName string) {
	best := ""
	for _, pid := range known {
		if pid == "" {
			continue
		}
		if strings.HasPrefix(base, pid+"-") && len(pid) > len(best) {
			best = pid
		}
	}
	if best != "" {
		return best, base[len(best)+1:]
	}
	if strings.HasPrefix(base, noneProjectName+"-") {
		return "", strings.TrimPrefix(base, noneProjectName+"-")
	}
	// 启发式：「分类-项目-问题名称」按前两个 "-" 拆（项目名含 "-" 时以已知列表为准）
	i := strings.Index(base, "-")
	if i < 0 {
		return "", base
	}
	j := strings.Index(base[i+1:], "-")
	if j < 0 {
		return "", base
	}
	j += i + 1
	return base[:j], base[j+1:]
}

// loadProblem 读取单个问题文件；不存在返回 nil。
// 文件名即身份：ID/Name/ProjectID 以文件名为准（防手改内容错位）。
func loadProblem(base string, known []string) *Problem {
	p := problemFilePath(base)
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) == 0 {
		return nil
	}
	raw = normalizeTimes(raw)
	pr := Problem{}
	if err := json.Unmarshal(raw, &pr); err != nil {
		_ = os.Rename(p, p+".bak")
		return nil
	}
	pr.ID = base
	pr.ProjectID, pr.Name = parseProblemID(base, known)
	if pr.Name == "" {
		pr.Name = base
	}
	return &pr
}

// saveProblem 原子写入问题文件（ID = 文件基名）
func saveProblem(pr *Problem) error {
	if pr == nil || pr.ID == "" {
		return fmt.Errorf("问题 ID 为空")
	}
	raw, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return err
	}
	dir := problemDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	p := problemFilePath(pr.ID)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// deleteProblemFile 删除问题文件（改名/删除问题时用）
func deleteProblemFile(id string) error {
	return os.Remove(problemFilePath(id))
}

// scanProblems 扫描问题排查目录，返回全部问题（按时间倒序由调用方排）
func scanProblems(known []string) []Problem {
	entries, err := os.ReadDir(problemDir())
	if err != nil {
		return []Problem{}
	}
	var out []Problem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		base := name[:len(name)-len(".json")]
		if base == "" {
			continue
		}
		if pr := loadProblem(base, known); pr != nil {
			out = append(out, *pr)
		}
	}
	return out
}

// ---------- 目录扫描（分类 / 项目的唯一来源） ----------

// scanCategories 扫描 exe 同目录，把「分类-项目.json」文件归并成分类树。
// 只认文件名含 "-" 的 .json 文件；其余（工作记录.json / 未指定.json / 杂项）跳过。
func scanCategories() []Category {
	entries, err := os.ReadDir(exeDir())
	if err != nil {
		return []Category{}
	}
	byCat := map[string]*Category{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		base := name[:len(name)-len(".json")]
		if !strings.Contains(base, "-") {
			continue
		}
		catName, projName := parseProjectID(base)
		if catName == "" || projName == "" {
			continue
		}
		c := byCat[catName]
		if c == nil {
			c = &Category{ID: catName, Name: catName, Projects: []Project{}}
			byCat[catName] = c
		}
		c.Projects = append(c.Projects, Project{ID: base, Name: projName})
	}
	cats := make([]Category, 0, len(byCat))
	for _, c := range byCat {
		sort.SliceStable(c.Projects, func(i, j int) bool {
			return naturalLess(c.Projects[i].Name, c.Projects[j].Name)
		})
		cats = append(cats, *c)
	}
	sort.SliceStable(cats, func(i, j int) bool { return naturalLess(cats[i].Name, cats[j].Name) })
	return cats
}

// naturalLess 自然排序：数字段按数值比较（2 < 10 < 124A），其余按字节
func naturalLess(a, b string) bool {
	i, j := 0, 0
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	lower := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 32
		}
		return c
	}
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		la, lb := lower(a[i]), lower(b[j])
		if la != lb {
			return la < lb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

// ---------- 主配置存取（仅设置） ----------

func loadData() (*StoreData, error) {
	d := &StoreData{
		Version:         1,
		FilterProjectID: FilterAll,
	}
	raw, err := os.ReadFile(masterPath())
	if err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, d); err != nil {
			// 文件损坏：备份后重建，绝不丢原始数据
			_ = os.Rename(masterPath(), masterPath()+".bak")
			d = &StoreData{Version: 1, FilterProjectID: FilterAll}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return d, err
	}
	if d.FilterProjectID == "" {
		d.FilterProjectID = FilterAll
	}
	if d.ProjectAliases == nil {
		d.ProjectAliases = map[string][]string{}
	}
	// 分类树以磁盘文件为准
	d.Categories = scanCategories()
	d.pruneAliasEntries()
	d.validateFilter()
	return d, nil
}

// validateFilter 筛选指向的分类/项目已不存在时重置为全部，避免界面上筛选悬空
func (d *StoreData) validateFilter() {
	if d.FilterProjectID == FilterAll {
		return
	}
	if cid, isCat := parseCatFilter(d.FilterProjectID); isCat {
		for _, c := range d.Categories {
			if c.ID == cid {
				return
			}
		}
	} else if _, _, ok := d.findProjectFull(d.FilterProjectID); ok {
		return
	}
	d.FilterProjectID = FilterAll
}

func saveData(d *StoreData) error {
	d.Version = 1
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := masterPath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, masterPath()) // 原子替换，防写一半断电损坏
}

// ---------- 项目记录文件存取 ----------

// loadProjectItems 读取单个项目的记录文件；不存在视为空。
// 加载时把每条记录的 ProjectID 归一为文件基名（防手改文件名后错位）。
func loadProjectItems(id string) []Item {
	p := projectFilePath(id)
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) == 0 {
		if id == "" {
			return loadLegacyNoneItems() // 兼容旧版 记录_未指定.json
		}
		return []Item{}
	}
	raw = normalizeTimes(raw)
	ps := ProjectStore{}
	if err := json.Unmarshal(raw, &ps); err != nil {
		// 损坏：备份后按空处理，绝不丢原始数据
		_ = os.Rename(p, p+".bak")
		return []Item{}
	}
	if ps.Items == nil {
		ps.Items = []Item{}
	}
	for i := range ps.Items {
		ps.Items[i].ProjectID = id
	}
	return ps.Items
}

// loadLegacyNoneItems 旧版未指定记录文件只读兼容（新写入一律 未指定.json）
func loadLegacyNoneItems() []Item {
	raw, err := os.ReadFile(filepath.Join(exeDir(), legacyNoneFileName))
	if err != nil || len(raw) == 0 {
		return []Item{}
	}
	raw = normalizeTimes(raw)
	ps := ProjectStore{}
	if err := json.Unmarshal(raw, &ps); err != nil || ps.Items == nil {
		return []Item{}
	}
	return ps.Items
}

// timeNoZone 匹配无时区后缀的时间戳（如 2026-01-02T15:04:05"，秒后直接跟引号），
// 补 Z 使其可被 time.Time 解析；带时区的时间戳秒后是 Z 或 ±hh:mm，不会命中
var timeNoZone = regexp.MustCompile(`("(?:time|doneTime)"\s*:\s*"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?)"`)

// normalizeTimes 兼容第三方工具生成的无时区时间戳（按 UTC 解析）
func normalizeTimes(raw []byte) []byte {
	return timeNoZone.ReplaceAll(raw, []byte(`${1}Z"`))
}

// saveProjectItems 原子写入单个项目的记录文件
func saveProjectItems(id string, items []Item) error {
	if items == nil {
		items = []Item{}
	}
	for i := range items {
		items[i].ProjectID = id
	}
	_, projName := parseProjectID(id)
	if id == "" {
		projName = noneProjectName
	}
	ps := ProjectStore{Version: 1, Project: projName, Items: items}
	raw, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	p := projectFilePath(id)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ensureProjectFile 新建项目时立即创建其记录文件（已存在则不动，沿用原文件）
func ensureProjectFile(id string) bool {
	p := projectFilePath(id)
	if _, err := os.Stat(p); err == nil {
		return false
	}
	return saveProjectItems(id, []Item{}) == nil
}

// ---------- 业务辅助 ----------

// itemProjects 记录归属的项目列表（兼容旧数据：projectIds 为空时按 projectId 单项目）
func itemProjects(it Item) []string {
	if len(it.ProjectIDs) > 0 {
		return it.ProjectIDs
	}
	return []string{it.ProjectID}
}

// probProjects 问题归属的项目列表（同上兼容逻辑）
func probProjects(p Problem) []string {
	if len(p.ProjectIDs) > 0 {
		return p.ProjectIDs
	}
	return []string{p.ProjectID}
}

// setItemProjects 设置记录的项目列表：首个作为主项目（决定存到哪个文件），去重
func setItemProjects(it *Item, ids []string) {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	it.ProjectIDs = out
	it.ProjectID = ""
	if len(out) > 0 {
		it.ProjectID = out[0]
	}
}

// setProbProjects 设置问题的项目列表：首个作为主项目（决定文件名）
func setProbProjects(p *Problem, ids []string) {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	p.ProjectIDs = out
	p.ProjectID = ""
	if len(out) > 0 {
		p.ProjectID = out[0]
	}
}

// matchProjectFilter 判断任一归属项目命中当前筛选（项目或分类）
func (d *StoreData) matchProjectFilter(pids []string) bool {
	if d.FilterProjectID == FilterAll {
		return true
	}
	if cid, isCat := parseCatFilter(d.FilterProjectID); isCat {
		for _, pid := range pids {
			if d.projectInCategory(pid, cid) {
				return true
			}
		}
		return false
	}
	for _, pid := range pids {
		if pid == d.FilterProjectID {
			return true
		}
	}
	return false
}

// projectNamesText 多个项目名的拼接（搜索与标签显示用；搜索会追加别名）
func (d *StoreData) projectNamesText(pids []string) []string {
	out := make([]string, 0, len(pids))
	for _, pid := range pids {
		out = append(out, d.projectName(pid))
		out = append(out, d.aliasesOf(pid)...) // 别名参与搜索命中
	}
	if len(out) == 0 {
		out = append(out, noneProjectName)
	}
	return out
}

// projectName 取项目显示名
func (d *StoreData) projectName(pid string) string {
	if pid == "" {
		return noneProjectName
	}
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			if p.ID == pid {
				return p.Name
			}
		}
	}
	// 文件已删但内存里还有残留时，直接按文件名反解，别显示"已删除"
	if _, proj := parseProjectID(pid); proj != "" {
		return proj
	}
	return "已删除的项目"
}

// allProjectIDs 收集全部项目 ID（含空 = 未指定）
func (d *StoreData) allProjectIDs() []string {
	ids := []string{""}
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// categoryNameOfProject 取项目所属分类名
func (d *StoreData) categoryNameOfProject(pid string) string {
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			if p.ID == pid {
				return c.Name
			}
		}
	}
	return ""
}

// findProject 查项目，返回分类与项目指针（不存在返回 nil）
func (d *StoreData) findProject(pid string) (*Category, *Project) {
	c, p, ok := d.findProjectFull(pid)
	if !ok {
		return nil, nil
	}
	return c, p
}

// findProjectFull 查项目，返回分类指针、项目指针与是否存在
func (d *StoreData) findProjectFull(pid string) (*Category, *Project, bool) {
	for i := range d.Categories {
		for j := range d.Categories[i].Projects {
			if d.Categories[i].Projects[j].ID == pid {
				return &d.Categories[i], &d.Categories[i].Projects[j], true
			}
		}
	}
	return nil, nil, false
}

// findProjectByName 按名称精确查找项目（跨分类），返回项目 ID。
// 别名同样精确命中（用户在项目号框里输入别名也能自动归到对应项目）。
func (d *StoreData) findProjectByName(name string) (string, bool) {
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			if p.Name == name {
				return p.ID, true
			}
			for _, a := range d.aliasesOf(p.ID) {
				if a == name {
					return p.ID, true
				}
			}
		}
	}
	return "", false
}

// aliasesOf 取项目别名列表
func (d *StoreData) aliasesOf(pid string) []string {
	if d.ProjectAliases == nil {
		return nil
	}
	return d.ProjectAliases[pid]
}

// setProjectAliases 设置项目别名（空列表即清除）；落盘到 工作记录.json
func (d *StoreData) setProjectAliases(pid string, aliases []string) {
	if d.ProjectAliases == nil {
		d.ProjectAliases = map[string][]string{}
	}
	if len(aliases) == 0 {
		delete(d.ProjectAliases, pid)
	} else {
		d.ProjectAliases[pid] = aliases
	}
}

// pruneAliasEntries 清掉已删项目的别名残余（项目文件消失 = 项目删除）
func (d *StoreData) pruneAliasEntries() {
	if d.ProjectAliases == nil {
		return
	}
	for pid := range d.ProjectAliases {
		if _, _, ok := d.findProjectFull(pid); !ok {
			delete(d.ProjectAliases, pid)
		}
	}
}

// projectMatches 项目名或任一别名是否命中关键词（项目筛选/候选面板搜索用）
func (d *StoreData) projectMatches(pid, name, kw string) bool {
	if kw == "" {
		return true
	}
	if containsFold(name, kw) {
		return true
	}
	for _, a := range d.aliasesOf(pid) {
		if containsFold(a, kw) {
			return true
		}
	}
	return false
}

// parseAliasList 把别名输入框文本拆成列表（支持 、 ， , ; 分号/空格分隔），去重去空
func parseAliasList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '、', '，', ',', ';', '；', '\t', '\n', '\r', ' ':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// projectInCategory 判断项目是否属于指定分类
func (d *StoreData) projectInCategory(pid, cid string) bool {
	for _, c := range d.Categories {
		if c.ID != cid {
			continue
		}
		for _, p := range c.Projects {
			if p.ID == pid {
				return true
			}
		}
	}
	return false
}

// categoryName 取分类显示名
func (d *StoreData) categoryName(cid string) string {
	for _, c := range d.Categories {
		if c.ID == cid {
			return c.Name
		}
	}
	return ""
}

// projectCategoryID 取项目所属分类 ID（不属于任何分类返回 ""）
func (d *StoreData) projectCategoryID(pid string) string {
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			if p.ID == pid {
				return c.ID
			}
		}
	}
	return ""
}

// ---------- 排序与过滤 ----------

// defaultRecentDays 默认只显示最近 5 个自然日（含今天）的记录，
// 更早的记录点「加载更早」逐段展开（main.go app.recentDays += 5）
const defaultRecentDays = 5

// FilterState 界面过滤条件：关键词（空格分隔多词 AND）+ 日期区间
type FilterState struct {
	Keyword  string
	DateFrom string // YYYY-MM-DD，空 = 不限
	DateTo   string // YYYY-MM-DD，空 = 不限
	// RecentDays 最近 N 个自然日窗口（含今天）：
	// 0 = 默认 defaultRecentDays 天；负数 = 不限（用于统计被窗口隐藏的更早记录条数）。
	// 关键词/日期搜索模式（Active）下窗口不生效，搜全部历史。
	RecentDays int
}

// Active 是否处于"全量搜索"模式（关键词或日期任一生效）
func (f *FilterState) Active() bool {
	return f.Keyword != "" || f.DateFrom != "" || f.DateTo != ""
}

// recentDays 取实际生效的天数窗口（0 视为默认值；负数 = 不限）
func (f *FilterState) recentDays() int {
	if f.RecentDays == 0 {
		return defaultRecentDays
	}
	return f.RecentDays
}

// withinRecentDays t 是否落在最近 n 个自然日内（n<=0 不限）。
// 按自然日截断：今天 00:00 起往前 n 天都算（n=5 即今天 + 前 4 天）。
func withinRecentDays(t time.Time, n int) bool {
	if n <= 0 {
		return true
	}
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).
		AddDate(0, 0, -(n - 1))
	return !t.Before(cutoff)
}

// matchItem 判断单条记录是否命中过滤条件（模糊匹配：大小写不敏感子串；多关键词 AND）
func (d *StoreData) matchItem(it Item, f FilterState) bool {
	// 关键词：文本 / 项目名 / 链接名与路径，多词全部命中才算
	if f.Keyword != "" {
		hay := []string{it.Text}
		hay = append(hay, d.projectNamesText(itemProjects(it))...)
		for _, l := range it.Links {
			hay = append(hay, l.Name, l.Path)
		}
		for _, st := range it.SubTodos {
			hay = append(hay, st.Text)
		}
		if !matchKeywords(hay, f.Keyword) {
			return false
		}
	}
	// 日期区间（按记录时间，含当天边界）
	if f.DateFrom != "" || f.DateTo != "" {
		if !dateInRange(it.Time, f) {
			return false
		}
	}
	return true
}

// matchProblem 问题排查命中过滤：问题名/档案/纪实/链接/项目名 + 日期区间
func (d *StoreData) matchProblem(p Problem, f FilterState) bool {
	if f.Keyword != "" {
		hay := []string{p.Name, p.ProfileText}
		hay = append(hay, d.projectNamesText(probProjects(p))...)
		for _, l := range p.ProfileLinks {
			hay = append(hay, l.Name, l.Path)
		}
		for _, e := range p.Log {
			hay = append(hay, e.Text)
			for _, l := range e.Links {
				hay = append(hay, l.Name, l.Path)
			}
		}
		if !matchKeywords(hay, f.Keyword) {
			return false
		}
	}
	if f.DateFrom != "" || f.DateTo != "" {
		if !dateInRange(p.Time, f) {
			return false
		}
	}
	return true
}

func matchKeywords(hay []string, keyword string) bool {
	for _, kw := range strings.Fields(keyword) {
		hit := false
		for _, h := range hay {
			if containsFold(h, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func dateInRange(t time.Time, f FilterState) bool {
	day := t.In(time.Local).Format("2006-01-02")
	if f.DateFrom != "" && day < f.DateFrom {
		return false
	}
	if f.DateTo != "" && day > f.DateTo {
		return false
	}
	return true
}

// ---------- 统一列表（记事/待办 + 问题排查） ----------
//
// 列表分两个展示区（main.go renderList）：
//
//	待办区：未完成待办（紧急 → 普通 → 非紧急收纳），每行两个网格
//	试机记录区：记事/已完成待办/问题排查，按天分组（按时间倒序自动聚天）
//
// 排序分组（越小越靠前）：
//
//	0 置顶记事
//	1 未完成紧急待办
//	2 未完成普通待办
//	3 未完成非紧急待办
//	4 记录流（记事、已完成待办、问题排查）：按时间倒序，同一天自然相邻
//
// 组内：待办按截止日期近优先再时间倒序；记录流按时间倒序（已完成待办按完成时间）。

// ListRow 列表一行：普通记录或问题排查（二选一）
type ListRow struct {
	Item *Item
	Prob *Problem
}

const (
	grpPinnedNote = 0
	grpUrgentTodo = 1
	grpNormalTodo = 2
	grpLowTodo    = 3 // 非紧急待办：排在普通待办之后
	grpFeed       = 4 // 记录流：记事/已完成待办/问题，按时间倒序
)

func rowGroup(r ListRow) int {
	if r.Prob != nil {
		return grpFeed
	}
	it := r.Item
	switch {
	case it.Type == TypeNote && it.Pinned:
		return grpPinnedNote
	case it.Type == TypeTodo && !it.Done && it.Urgent:
		return grpUrgentTodo
	case it.Type == TypeTodo && !it.Done && it.Low:
		return grpLowTodo
	case it.Type == TypeTodo && !it.Done:
		return grpNormalTodo
	default:
		// 记事与已完成待办进入记录流，按时间倒序参与按天分组
		return grpFeed
	}
}

// isLowPendingRow 判断该行是否是「未完成的非紧急待办」（默认收纳的对象）
func isLowPendingRow(r ListRow) bool {
	return r.Item != nil && r.Item.Type == TypeTodo && !r.Item.Done && r.Item.Low && !r.Item.Urgent
}

// isPendingTodoRow 未完成待办（待办区）
func isPendingTodoRow(r ListRow) bool {
	return r.Item != nil && r.Item.Type == TypeTodo && !r.Item.Done
}

// isPinnedNoteRow 置顶记事（列表最顶，不受天数窗口限制）
func isPinnedNoteRow(r ListRow) bool {
	return r.Item != nil && r.Item.Type == TypeNote && r.Item.Pinned
}

// isFeedRow 记录流行（试机记录区：记事/已完成待办/问题）
func isFeedRow(r ListRow) bool {
	return !isPendingTodoRow(r) && !isPinnedNoteRow(r)
}

// lessRow 组内比较：待办按截止日期近优先；记录流按时间倒序（同天相邻，支撑按天分组）
func lessRow(a, b ListRow) bool {
	ga, gb := rowGroup(a), rowGroup(b)
	if ga != gb {
		return ga < gb
	}
	// 待办组内：截止日期近的在前
	if ga == grpUrgentTodo || ga == grpNormalTodo || ga == grpLowTodo {
		ad, bd := "", ""
		if a.Item != nil {
			ad = a.Item.DueDate
		}
		if b.Item != nil {
			bd = b.Item.DueDate
		}
		if ad != "" && bd != "" && ad != bd {
			return ad < bd
		}
		if (ad == "") != (bd == "") {
			return ad != ""
		}
	}
	return rowTime(a).After(rowTime(b))
}

func rowTime(r ListRow) time.Time {
	if r.Prob != nil {
		return r.Prob.Time
	}
	// 已完成的待办按完成时间参与排序（未完成、或完成时间缺失时按记录时间）
	if r.Item.Done && !r.Item.DoneTime.IsZero() {
		return r.Item.DoneTime
	}
	return r.Item.Time
}

// visibleRows 产出统一列表（记事/待办 + 问题排查）。
// 过滤：默认只显示当前项目最近 defaultRecentDays 个自然日（RecentDays 可扩展）；
// f.Active() 时搜全部项目全量记录（时间窗口不生效）。
// 不受时间窗口限制的行：置顶记事、未完成待办、未解决的问题排查（都是"一直要看"的内容）。
func (d *StoreData) visibleRows(items []Item, probs []Problem, filter FilterState) []ListRow {
	rows := []ListRow{}

	passProjectTime := func(pids []string, t time.Time, alwaysVisible bool) bool {
		if filter.Active() {
			return true
		}
		if !d.matchProjectFilter(pids) {
			return false
		}
		if alwaysVisible {
			return true
		}
		return withinRecentDays(t, filter.recentDays())
	}

	for i := range items {
		it := items[i]
		if filter.Active() {
			if !d.matchItem(it, filter) {
				continue
			}
		} else {
			// 置顶记事与未完成待办不受天数窗口限制（不能因为创建得早就消失）
			always := it.Pinned || (it.Type == TypeTodo && !it.Done)
			if !passProjectTime(itemProjects(it), it.Time, always) {
				continue
			}
		}
		rows = append(rows, ListRow{Item: &it})
	}
	for i := range probs {
		p := probs[i]
		if filter.Active() {
			if !d.matchProblem(p, filter) {
				continue
			}
		} else if !passProjectTime(probProjects(p), p.Time, !p.Resolved) {
			continue
		}
		rows = append(rows, ListRow{Prob: &p})
	}

	sort.SliceStable(rows, func(i, j int) bool { return lessRow(rows[i], rows[j]) })
	return rows
}

// visibleItems 只看记事/待办的视图（单元测试与兼容用）
func (d *StoreData) visibleItems(items []Item, filter FilterState) []Item {
	rows := d.visibleRows(items, nil, filter)
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		if r.Item != nil {
			out = append(out, *r.Item)
		}
	}
	return out
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) &&
		indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	// 简单的大小写不敏感查找（ASCII 折叠，够用）
	ls := toLowerASCII(s)
	lsub := toLowerASCII(sub)
	return indexOf(ls, lsub)
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 || m > n {
		return -1
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

// weekName 返回中文星期，如「周三」（按本地时区，避免 Z 时间戳跨日错位）
func weekName(t time.Time) string {
	return "周" + []string{"日", "一", "二", "三", "四", "五", "六"}[int(t.In(time.Local).Weekday())]
}

// dayKey 按天分组的键（本地时区 YYYY-MM-DD）
func dayKey(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02")
}

// dayLabel 天分组标签：「今天 10-24 周五」「昨天 10-23 周四」「10-22 周三」「2025-12-01 周一」
func dayLabel(t time.Time) string {
	lt := t.In(time.Local)
	w := weekName(lt)
	switch dayKey(lt) {
	case dayKey(time.Now()):
		return "今天 " + lt.Format("01-02") + " " + w
	case dayKey(time.Now().AddDate(0, 0, -1)):
		return "昨天 " + lt.Format("01-02") + " " + w
	}
	if lt.After(time.Now().AddDate(0, 0, -6)) {
		return lt.Format("01-02") + " " + w
	}
	return lt.Format("2006-01-02") + " " + w
}

func fmtTime(t time.Time) string {
	lt := t.In(time.Local)
	now := time.Now()
	w := weekName(lt)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch {
	case !lt.Before(today):
		return "今天 " + lt.Format("15:04") + " " + w
	case lt.After(today.Add(-24 * time.Hour)):
		return "昨天 " + lt.Format("15:04") + " " + w
	case lt.After(today.Add(-7 * 24 * time.Hour)):
		return lt.Format("01-02 15:04") + " " + w
	default:
		return lt.Format("2006-01-02 15:04") + " " + w
	}
}
