package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *StoreData {
	t.Helper()
	return &StoreData{
		Version: 1, FilterProjectID: FilterAll,
		Categories: []Category{{
			ID: "分类", Name: "分类",
			Projects: []Project{{ID: "分类-项目A", Name: "项目A"}, {ID: "分类-项目B", Name: "项目B"}},
		}},
	}
}

// useTempDir 把 exe 目录指到临时目录
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := exeDirFn
	exeDirFn = func() (string, error) { return filepath.Join(dir, "app.exe"), nil }
	t.Cleanup(func() { exeDirFn = orig })
	return dir
}

func mkItem(id, typ, text, pid string, t time.Time) Item {
	return Item{ID: id, Type: typ, Text: text, ProjectID: pid, Time: t}
}

// 1. 项目分文件：保存/读取往返一致（文件名 = 分类-项目.json）
func TestProjectFileRoundTrip(t *testing.T) {
	dir := useTempDir(t)

	items := []Item{mkItem("1", TypeNote, "hello 世界", "研发-124", time.Now())}
	if err := saveProjectItems("研发-124", items); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "研发-124.json")); err != nil {
		t.Fatal("项目文件未生成:", err)
	}
	got := loadProjectItems("研发-124")
	if len(got) != 1 || got[0].Text != "hello 世界" {
		t.Fatalf("往返不一致: %+v", got)
	}
}

// 2. 文件名清洗 + 分类/项目 ID 构造与反解
func TestSanitizeFileName(t *testing.T) {
	if got := sanitizeFileName(`a/b\c:d*e?f"g<h>i|j`); strings.ContainsAny(got, `/\:*?"<>|`) {
		t.Fatal("清洗失败:", got)
	}
}

func TestBuildParseProjectID(t *testing.T) {
	id, ok := buildProjectID("研发", "124A")
	if !ok || id != "研发-124A" {
		t.Fatalf("构造 ID 失败: %q %v", id, ok)
	}
	cat, proj := parseProjectID(id)
	if cat != "研发" || proj != "124A" {
		t.Fatalf("反解失败: %q %q", cat, proj)
	}
	// 分类名里的 "-" 归一为 "_"，保证能反解
	id2, ok := buildProjectID("AI-研发", "124-2")
	if !ok || id2 != "AI_研发-124-2" {
		t.Fatalf("分类名连字符处理失败: %q", id2)
	}
	cat2, proj2 := parseProjectID(id2)
	if cat2 != "AI_研发" || proj2 != "124-2" {
		t.Fatalf("反解失败: %q %q", cat2, proj2)
	}
	// 空名不合法
	if _, ok := buildProjectID("", "x"); ok {
		t.Fatal("空分类应拒绝")
	}
	if _, ok := buildProjectID("x", "  "); ok {
		t.Fatal("空项目应拒绝")
	}
}

// 3. 目录扫描：分类/项目完全由「分类-项目.json」文件决定
func TestScanCategories(t *testing.T) {
	dir := useTempDir(t)
	for _, name := range []string{
		"研发-124.json", "研发-2.json", "研发-10.json",
		"测试-T01.json",
		"工作记录.json", "未指定.json", "记录_旧项目.json", "研发-坏.json.bak",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"version":1,"items":[]}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cats := scanCategories()
	if len(cats) != 2 {
		t.Fatalf("分类数不对: %+v", cats)
	}
	if cats[0].Name != "测试" || cats[1].Name != "研发" {
		t.Fatalf("分类排序不对: %s %s", cats[0].Name, cats[1].Name)
	}
	projs := cats[1].Projects
	if len(projs) != 3 {
		t.Fatalf("研发项目数不对: %+v", projs)
	}
	// 自然排序：2 < 10 < 124
	if projs[0].Name != "2" || projs[1].Name != "10" || projs[2].Name != "124" {
		t.Fatalf("项目排序不对: %s %s %s", projs[0].Name, projs[1].Name, projs[2].Name)
	}
	if projs[2].ID != "研发-124" {
		t.Fatalf("项目 ID 应为文件基名: %s", projs[2].ID)
	}
}

// 4. 读取时把记录的 ProjectID 归一为文件基名（防手改文件名后错位）
func TestLoadNormalizesProjectID(t *testing.T) {
	useTempDir(t)
	raw := `{"version":1,"project":"旧名","items":[{"id":"1","type":"note","text":"x","projectId":"别的","time":"2026-01-02T15:04:05+08:00"}]}`
	if err := os.WriteFile(filepath.Join(exeDir(), "研发-124.json"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	got := loadProjectItems("研发-124")
	if len(got) != 1 || got[0].ProjectID != "研发-124" {
		t.Fatalf("ProjectID 未归一: %+v", got)
	}
}

// 5. 无时区时间戳宽容解析
func TestNormalizeTimes(t *testing.T) {
	raw := []byte(`{"version":1,"project":"p","items":[{"id":"1","type":"note","text":"x","projectId":"","time":"2026-01-02T15:04:05"}]}`)
	out := normalizeTimes(raw)
	ps := ProjectStore{}
	if err := json.Unmarshal(out, &ps); err != nil {
		t.Fatal("无时区解析失败:", err)
	}
	// 带时区不受影响
	raw2 := []byte(`{"items":[{"id":"1","time":"2026-01-02T15:04:05+08:00"}]}`)
	out2 := normalizeTimes(raw2)
	ps2 := ProjectStore{}
	if err := json.Unmarshal(out2, &ps2); err != nil {
		t.Fatal("带时区解析失败:", err)
	}
	if want := "2026-01-02T15:04:05+08:00"; ps2.Items[0].Time.Format("2006-01-02T15:04:05-07:00") != want {
		t.Fatalf("带时区时间被改动: %s", ps2.Items[0].Time)
	}
}

// 6. 模糊搜索：子串、大小写不敏感、多关键词 AND、跨项目、命中项目名/链接
func TestSearchFuzzy(t *testing.T) {
	d := testStore(t)
	now := time.Now()
	items := []Item{
		mkItem("1", TypeNote, "修复登录Bug", "分类-项目A", now),
		mkItem("2", TypeNote, "香蕉计划", "分类-项目B", now),
		{ID: "3", Type: TypeNote, Text: "资料", ProjectID: "分类-项目A", Time: now,
			Links: []Link{{Name: "设计稿", Path: `D:\docs\spec.pdf`}}},
	}
	all := d.visibleItems(items, FilterState{Keyword: "香蕉"})
	if len(all) != 1 || all[0].ID != "2" {
		t.Fatalf("关键词搜索失败: %+v", all)
	}
	// 大小写不敏感
	all = d.visibleItems(items, FilterState{Keyword: "bug"})
	if len(all) != 1 || all[0].ID != "1" {
		t.Fatalf("大小写搜索失败: %+v", all)
	}
	// 命中链接路径
	all = d.visibleItems(items, FilterState{Keyword: "spec.PDF"})
	if len(all) != 1 || all[0].ID != "3" {
		t.Fatalf("链接搜索失败: %+v", all)
	}
	// 命中项目名
	all = d.visibleItems(items, FilterState{Keyword: "项目B"})
	if len(all) != 1 || all[0].ID != "2" {
		t.Fatalf("项目名搜索失败: %+v", all)
	}
	// 多词 AND
	all = d.visibleItems(items, FilterState{Keyword: "香蕉 计划"})
	if len(all) != 1 {
		t.Fatalf("多词 AND 失败: %+v", all)
	}
	all = d.visibleItems(items, FilterState{Keyword: "香蕉 bug"})
	if len(all) != 0 {
		t.Fatalf("多词 AND 应无结果: %+v", all)
	}
}

// 7. 日期区间过滤（含边界）
func TestDateFilter(t *testing.T) {
	d := testStore(t)
	items := []Item{
		mkItem("1", TypeNote, "a", "分类-项目A", time.Date(2026, 10, 1, 10, 0, 0, 0, time.Local)),
		mkItem("2", TypeNote, "b", "分类-项目A", time.Date(2026, 10, 5, 23, 0, 0, 0, time.Local)),
		mkItem("3", TypeNote, "c", "分类-项目B", time.Date(2026, 10, 10, 8, 0, 0, 0, time.Local)),
	}
	all := d.visibleItems(items, FilterState{DateFrom: "2026-10-02", DateTo: "2026-10-10"})
	if len(all) != 2 {
		t.Fatalf("日期区间过滤失败: %+v", all)
	}
	all = d.visibleItems(items, FilterState{DateFrom: "2026-10-05", DateTo: "2026-10-05"})
	if len(all) != 1 || all[0].ID != "2" {
		t.Fatalf("单日过滤失败: %+v", all)
	}
	// 关键词 + 日期 组合
	all = d.visibleItems(items, FilterState{Keyword: "c", DateFrom: "2026-10-09", DateTo: "2026-10-11"})
	if len(all) != 1 || all[0].ID != "3" {
		t.Fatalf("组合过滤失败: %+v", all)
	}
	all = d.visibleItems(items, FilterState{Keyword: "a", DateFrom: "2026-10-09", DateTo: "2026-10-11"})
	if len(all) != 0 {
		t.Fatalf("组合过滤应为空: %+v", all)
	}
}

// 8. 搜索模式跨项目且不受默认时间窗口限制
func TestSearchIgnoresWeekWindowAndProjectFilter(t *testing.T) {
	d := testStore(t)
	d.FilterProjectID = "分类-项目B" // 界面筛选在项目B
	old := time.Now().AddDate(0, -3, 0)
	items := []Item{
		mkItem("1", TypeNote, "三个月前的项目A记录", "分类-项目A", old),
		mkItem("2", TypeNote, "项目B最近记录", "分类-项目B", time.Now()),
	}
	all := d.visibleItems(items, FilterState{Keyword: "项目A"})
	if len(all) != 1 || all[0].ID != "1" {
		t.Fatalf("全量搜索应跨项目跨时间: %+v", all)
	}
	// 非搜索模式：项目筛选 + 一周窗口 → 只剩项目B新记录
	all = d.visibleItems(items, FilterState{})
	if len(all) != 1 || all[0].ID != "2" {
		t.Fatalf("默认过滤失败: %+v", all)
	}
	// 分类筛选：cat:分类 覆盖该分类下全部项目
	d.FilterProjectID = catFilterValue("分类")
	all = d.visibleItems(items, FilterState{})
	if len(all) != 1 || all[0].ID != "2" {
		t.Fatalf("分类过滤失败: %+v", all)
	}
}

// 9. 最近 5 天窗口：普通记录随窗，置顶/未完成待办/未解决问题豁免，RecentDays 负数不限
func TestRecentDaysWindow(t *testing.T) {
	d := testStore(t)
	now := time.Now()
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	items := []Item{
		mkItem("n-today", TypeNote, "今天记事", "分类-项目A", day(0)),
		mkItem("n-edge", TypeNote, "四天前记事", "分类-项目A", day(4)),
		mkItem("n-out", TypeNote, "五天前记事", "分类-项目A", day(5)),
		{ID: "n-oldpin", Type: TypeNote, Text: "旧置顶", ProjectID: "分类-项目A", Time: day(30), Pinned: true},
		{ID: "t-old", Type: TypeTodo, Text: "旧待办", ProjectID: "分类-项目A", Time: day(30)},
		{ID: "t-olddone", Type: TypeTodo, Text: "旧完成", ProjectID: "分类-项目A", Time: day(30), Done: true},
	}
	// 默认 5 天：n-today/n-edge 在窗；n-out/n-olddone 随窗；n-oldpin、t-old 豁免
	all := d.visibleItems(items, FilterState{})
	want := map[string]bool{"n-today": true, "n-edge": true, "n-oldpin": true, "t-old": true}
	if len(all) != len(want) {
		t.Fatalf("默认窗口行数不对: %+v", all)
	}
	for _, it := range all {
		if !want[it.ID] {
			t.Fatalf("不该出现: %s", it.ID)
		}
	}
	// RecentDays 负数 = 不限，全量可见
	all = d.visibleItems(items, FilterState{RecentDays: -1})
	if len(all) != 6 {
		t.Fatalf("不限窗口应全量: %d", len(all))
	}
	// 扩到 7 天：n-out 进来
	all = d.visibleItems(items, FilterState{RecentDays: 7})
	found := false
	for _, it := range all {
		if it.ID == "n-out" {
			found = true
		}
	}
	if !found {
		t.Fatalf("扩展窗口应含 n-out: %+v", all)
	}
	// 自然日边界：今天 + 前 4 天共 5 天
	if !withinRecentDays(day(4), 5) || withinRecentDays(day(5), 5) {
		t.Fatal("5 天窗口边界不对")
	}
	if !withinRecentDays(day(999), 0) {
		t.Fatal("n<=0 应不限")
	}
	// 天键与标签
	if dayKey(day(0)) != time.Now().Format("2006-01-02") {
		t.Fatal("dayKey 应按自然日")
	}
	if s := dayLabel(day(0)); !strings.Contains(s, "今天") {
		t.Fatalf("dayLabel 今天: %s", s)
	}
	if s := dayLabel(day(1)); !strings.Contains(s, "昨天") {
		t.Fatalf("dayLabel 昨天: %s", s)
	}
}

func fmtS(i int) string { return "id" + strconv.Itoa(i) }

// 10. 未完成待办置顶与紧急优先
func TestPendingTop(t *testing.T) {
	d := testStore(t)
	now := time.Now()
	items := []Item{
		mkItem("n1", TypeNote, "旧记事", "分类-项目A", now.Add(-2*time.Hour)),
		{ID: "t1", Type: TypeTodo, Text: "普通待办", ProjectID: "分类-项目A", Time: now.Add(-time.Hour)},
		{ID: "t2", Type: TypeTodo, Text: "紧急待办", ProjectID: "分类-项目A", Time: now.Add(-2 * time.Hour), Urgent: true},
		mkItem("n2", TypeNote, "新记事", "分类-项目A", now),
	}
	all := d.visibleItems(items, FilterState{})
	if len(all) != 4 || all[0].ID != "t2" || all[1].ID != "t1" || all[2].ID != "n2" {
		t.Fatalf("置顶排序失败: %+v", all)
	}
}

// 11. 时间显示带星期
func TestFmtTimeWeekday(t *testing.T) {
	for _, tc := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local), "周三"},
		{time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local), "周日"},
	} {
		if got := weekName(tc.t); got != tc.want {
			t.Fatalf("weekName(%s)=%s want %s", tc.t, got, tc.want)
		}
		if s := fmtTime(tc.t); !strings.Contains(s, tc.want) {
			t.Fatalf("fmtTime(%s)=%s 缺少星期", tc.t, s)
		}
	}
	// 今天/昨天分支也带星期
	today := fmtTime(time.Now())
	if !strings.Contains(today, "今天") || !strings.Contains(today, weekName(time.Now())) {
		t.Fatalf("今天格式缺星期: %s", today)
	}
}

// 12. 筛选悬空自动重置
func TestValidateFilter(t *testing.T) {
	d := testStore(t)
	d.FilterProjectID = "分类-不存在"
	d.validateFilter()
	if d.FilterProjectID != FilterAll {
		t.Fatalf("悬空项目筛选应重置: %s", d.FilterProjectID)
	}
	d.FilterProjectID = catFilterValue("没这个分类")
	d.validateFilter()
	if d.FilterProjectID != FilterAll {
		t.Fatalf("悬空分类筛选应重置: %s", d.FilterProjectID)
	}
	d.FilterProjectID = "分类-项目A"
	d.validateFilter()
	if d.FilterProjectID != "分类-项目A" {
		t.Fatal("有效筛选不应被重置")
	}
}

// 13. 问题排查文件名构造与反解
func TestProblemIDRoundTrip(t *testing.T) {
	known := []string{"", "研发-124", "研发-124-2", "测试-T01"}
	// 普通项目
	id, ok := buildProblemID("研发-124", "屏幕闪烁")
	if !ok || id != "研发-124-屏幕闪烁" {
		t.Fatalf("问题 ID 构造失败: %q %v", id, ok)
	}
	pid, name := parseProblemID(id, known)
	if pid != "研发-124" || name != "屏幕闪烁" {
		t.Fatalf("反解失败: %q %q", pid, name)
	}
	// 项目名含 "-"：最长前缀匹配
	id2, _ := buildProblemID("研发-124-2", "花屏")
	pid2, name2 := parseProblemID(id2, known)
	if pid2 != "研发-124-2" || name2 != "花屏" {
		t.Fatalf("长项目名反解失败: %q %q", pid2, name2)
	}
	// 未指定
	id3, _ := buildProblemID("", "杂项问题")
	if id3 != "未指定-杂项问题" {
		t.Fatalf("未指定 ID 构造失败: %q", id3)
	}
	pid3, name3 := parseProblemID(id3, known)
	if pid3 != "" || name3 != "杂项问题" {
		t.Fatalf("未指定反解失败: %q %q", pid3, name3)
	}
	// 问题名含 "-"：构造时归一为 "_"，保证可反解
	id4, _ := buildProblemID("测试-T01", "A-B 测试")
	if strings.Contains(id4[len("测试-T01-"):], "-") {
		t.Fatalf("问题名连字符未归一: %q", id4)
	}
	// 空问题名拒绝
	if _, ok := buildProblemID("研发-124", "  "); ok {
		t.Fatal("空问题名应拒绝")
	}
}

// 14. 问题文件读写往返（独立目录）
func TestProblemFileRoundTrip(t *testing.T) {
	dir := useTempDir(t)
	pr := Problem{
		ID: "研发-124-屏幕闪烁", Name: "屏幕闪烁", ProjectID: "研发-124",
		Time:      time.Date(2026, 10, 22, 10, 0, 0, 0, time.Local),
		ProfileText: "偶发闪烁",
		ProfileLinks: []Link{{Name: "日志", Path: `D:/logs/a.log`}},
		Log: []ProblemEntry{{ID: "e1", Text: "重启驱动后复现", Time: time.Now()}},
	}
	if err := saveProblem(&pr); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "问题排查", "研发-124-屏幕闪烁.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatal("问题文件未生成:", err)
	}
	got := loadProblem("研发-124-屏幕闪烁", []string{"", "研发-124"})
	if got == nil || got.ProfileText != "偶发闪烁" || len(got.Log) != 1 {
		t.Fatalf("往返不一致: %+v", got)
	}
	if got.Name != "屏幕闪烁" || got.ProjectID != "研发-124" {
		t.Fatalf("文件名归一失败: %+v", got)
	}
	// 扫描
	all := scanProblems([]string{"", "研发-124"})
	if len(all) != 1 {
		t.Fatalf("扫描数量不对: %d", len(all))
	}
	// 删除
	deleteProblemFile("研发-124-屏幕闪烁")
	if len(scanProblems([]string{"", "研发-124"})) != 0 {
		t.Fatal("删除后仍能扫到")
	}
}

// 15. 统一排序：置顶记事 → 紧急待办 → 普通待办 → 记录流（时间倒序，同天相邻）
func TestUnifiedSortOrder(t *testing.T) {
	d := testStore(t)
	now := time.Now()
	items := []Item{
		{ID: "n-pin", Type: TypeNote, Text: "置顶", ProjectID: "分类-项目A", Time: now.Add(-5 * time.Hour), Pinned: true},
		{ID: "t-urgent", Type: TypeTodo, Text: "紧急", ProjectID: "分类-项目A", Time: now.Add(-4 * time.Hour), Urgent: true},
		{ID: "t-normal", Type: TypeTodo, Text: "普通待办", ProjectID: "分类-项目A", Time: now.Add(-3 * time.Hour)},
		{ID: "n-old", Type: TypeNote, Text: "旧记事", ProjectID: "分类-项目A", Time: now.Add(-2 * time.Hour)},
		{ID: "t-done", Type: TypeTodo, Text: "已完成", ProjectID: "分类-项目A", Time: now.Add(-time.Hour),
			Done: true, DoneTime: now.Add(-10 * time.Minute)},
	}
	probs := []Problem{
		{ID: "p-open", Name: "未解决问题", ProjectID: "分类-项目A", Time: now.Add(-90 * time.Minute)},
		{ID: "p-done", Name: "已解决问题", ProjectID: "分类-项目A", Time: now.Add(-30 * time.Minute), Resolved: true},
	}
	rows := d.visibleRows(items, probs, FilterState{})
	if len(rows) != 7 {
		t.Fatalf("行数不对: %d", len(rows))
	}
	// 待办区在前（未完成待办），记录流按时间倒序：t-done(10m) → p-done(30m) → p-open(90m) → n-old(2h)。
	// 记录流同天相邻，支撑按天分组。
	want := []string{"n-pin", "t-urgent", "t-normal", "t-done", "p-done", "p-open", "n-old"}
	got := make([]string, 0, 7)
	for _, r := range rows {
		if r.Item != nil {
			got = append(got, r.Item.ID)
		} else {
			got = append(got, r.Prob.ID)
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序不符\n got: %v\nwant: %v", got, want)
		}
	}
}

// 16. 问题关键词搜索 + 未解决问题不受时间窗口限制
func TestProblemSearchAndWindow(t *testing.T) {
	d := testStore(t)
	old := time.Now().AddDate(0, -2, 0)
	probs := []Problem{
		{ID: "p1", Name: "老问题", ProjectID: "分类-项目A", Time: old,
			ProfileText: "驱动崩溃", ProfileLinks: []Link{{Name: "dump", Path: `D:/dumps/x.dmp`}}},
		{ID: "p2", Name: "新问题", ProjectID: "分类-项目B", Time: time.Now(), Resolved: true},
		{ID: "p3", Name: "已解决老问题", ProjectID: "分类-项目A", Time: old, Resolved: true},
	}
	// 未解决超窗显示（p1）；已解决随窗口（p2 近=显示，p3 远=不显示）。
	// 记录流按时间倒序：p2（现在）在前，p1（老、未解决豁免窗口）在后。
	rows := d.visibleRows(nil, probs, FilterState{})
	if len(rows) != 2 || rows[0].Prob.ID != "p2" || rows[1].Prob.ID != "p1" {
		t.Fatalf("窗口规则不对: %+v %+v", rows[0].Prob, rows[1].Prob)
	}
	// 关键词命中档案文本
	rows = d.visibleRows(nil, probs, FilterState{Keyword: "崩溃"})
	if len(rows) != 1 || rows[0].Prob.ID != "p1" {
		t.Fatalf("档案搜索失败: %+v", rows)
	}
	// 关键词命中链接路径
	rows = d.visibleRows(nil, probs, FilterState{Keyword: "dmp"})
	if len(rows) != 1 {
		t.Fatalf("链接搜索失败: %+v", rows)
	}
	// 日期过滤
	rows = d.visibleRows(nil, probs, FilterState{DateFrom: time.Now().Format("2006-01-02")})
	if len(rows) != 1 || rows[0].Prob.ID != "p2" {
		t.Fatalf("日期过滤失败: %+v", rows)
	}
}

// 17. 非紧急待办：排在普通待办之后、记事之前，且可被收纳识别
func TestLowTodoSort(t *testing.T) {
	d := testStore(t)
	now := time.Now()
	items := []Item{
		{ID: "n-pin", Type: TypeNote, Text: "置顶", ProjectID: "分类-项目A", Time: now, Pinned: true},
		{ID: "t-urg", Type: TypeTodo, Text: "紧急", ProjectID: "分类-项目A", Time: now, Urgent: true},
		{ID: "t-norm", Type: TypeTodo, Text: "普通", ProjectID: "分类-项目A", Time: now},
		{ID: "t-low", Type: TypeTodo, Text: "非紧急", ProjectID: "分类-项目A", Time: now, Low: true},
		{ID: "n-1", Type: TypeNote, Text: "记事", ProjectID: "分类-项目A", Time: now},
		{ID: "t-low-done", Type: TypeTodo, Text: "非紧急已完成", ProjectID: "分类-项目A", Time: now, Low: true, Done: true},
	}
	rows := d.visibleRows(items, nil, FilterState{})
	want := []string{"n-pin", "t-urg", "t-norm", "t-low", "n-1", "t-low-done"}
	got := []string{}
	lowHits := 0
	for _, r := range rows {
		got = append(got, r.Item.ID)
		if isLowPendingRow(r) {
			lowHits++
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序不符\n got: %v\nwant: %v", got, want)
		}
	}
	// 只有「未完成的非紧急待办」是收纳对象（已完成的不算）
	if lowHits != 1 {
		t.Fatalf("收纳对象数量不对: %d", lowHits)
	}
	// 优先级取值
	if items[1].Priority() != "urgent" || items[2].Priority() != "normal" || items[3].Priority() != "low" {
		t.Fatal("优先级取值不对")
	}
	// 同时标 urgent 与 low 时以紧急为准
	both := Item{Type: TypeTodo, Urgent: true, Low: true}
	if both.Priority() != "urgent" {
		t.Fatal("紧急应优先于非紧急")
	}
	if isLowPendingRow(ListRow{Item: &both}) {
		t.Fatal("紧急项不应被当作收纳对象")
	}
}

// 18. 置顶 / 子待办 / 非紧急 字段随项目文件持久化
func TestItemFieldsRoundTrip(t *testing.T) {
	useTempDir(t)
	items := []Item{
		{ID: "1", Type: TypeNote, Text: "置顶记事", ProjectID: "研发-124", Time: time.Now(), Pinned: true},
		{ID: "2", Type: TypeTodo, Text: "非紧急待办", ProjectID: "研发-124", Time: time.Now(), Low: true,
			SubTodos: []SubTodo{{ID: "s1", Text: "子一"}, {ID: "s2", Text: "子二", Done: true}}},
	}
	if err := saveProjectItems("研发-124", items); err != nil {
		t.Fatal(err)
	}
	got := loadProjectItems("研发-124")
	if len(got) != 2 {
		t.Fatalf("条数不对: %d", len(got))
	}
	if !got[0].Pinned {
		t.Fatal("置顶字段丢失")
	}
	if !got[1].Low {
		t.Fatal("非紧急字段丢失")
	}
	if len(got[1].SubTodos) != 2 || !got[1].SubTodos[1].Done {
		t.Fatalf("子待办丢失: %+v", got[1].SubTodos)
	}
}
