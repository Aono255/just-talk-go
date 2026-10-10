package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/hotkey"
	"github.com/c/just-talk-go/internal/correction"
	"github.com/c/just-talk-go/plugins/voice"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	tStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	lStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	vStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	aStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	wStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	eStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	dStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).MarginTop(1)
)

type backendMsg hotkey.ProviderInfo
type devMsg struct {
	Devices []string
	Error   error
}
type fieldType int

const (
	fString fieldType = iota
	fToggle
	fSelect
)

type field struct {
	label   string
	key     string
	help    string
	fType   fieldType
	input   textinput.Model
	boolVal bool
	opts    []string
	optIdx  int
}

type Model struct {
	w, h        int
	ready       bool
	info        hotkey.ProviderInfo
	logs        []string
	devices     []string
	cfg         *config.Config
	debug       bool
	showLogs    bool
	OnSave      func(*config.Config) error
	fields      []field
	cursor      int
	editing     bool
	helpVisible bool
	logExpanded bool
}

func New(cfg *config.Config) *Model {
	vc := cfg.Voice
	cc := cfg.Correction
	ti := func(v string) textinput.Model { t := textinput.New(); t.SetValue(v); t.Cursor.Blink = false; return t }
	fs := []field{
		{label: "语音输入", key: "enabled", help: "关闭后不注册热键", fType: fToggle, boolVal: vc.Enabled},
		{label: "热键", key: "push_to_talk", help: "例: Alt+Super / F9 / Ctrl+Alt+Tab；不支持字母、数字、标点、空格等普通字符键", fType: fString, input: ti(vc.PushToTalk)},
		{label: "模式", key: "mode", help: "toggle 切换 / hold 按住", fType: fSelect, opts: []string{"toggle", "hold"}, optIdx: idxOf([]string{"toggle", "hold"}, vc.Mode)},
		{label: "App Key", key: "app_key", help: "火山 App ID", fType: fString, input: ti(vc.AppKey)},
		{label: "Access Key", key: "access_key", help: "火山 Access Token", fType: fString, input: ti(vc.AccessKey)},
		{label: "自动上屏", key: "auto_submit", help: "识别后自动粘贴", fType: fToggle, boolVal: vc.AutoSubmit},
		{label: "停止延迟(ms)", key: "stop_delay_ms", help: "松手后补录毫秒", fType: fString, input: ti(fmt.Sprintf("%d", vc.StopDelayMs))},
		{label: "热词", key: "hotwords", help: "逗号分隔术语", fType: fString, input: ti(strings.Join(vc.Hotwords, ", "))},
		{label: "火山热词表 ID", key: "boosting_table_id", help: "填写后识别使用云端词表；留空使用热词", fType: fString, input: ti(vc.BoostingTableID)},
		{label: "Codex 纠错", key: "correction_enabled", help: "macOS：结合当前聊天整理识别文字，再上屏", fType: fToggle, boolVal: cc.Enabled},
		{label: "AI 纠错术语", key: "correction_terms", help: "逗号分隔；仅供纠错模型参考，不传给语音识别", fType: fString, input: ti(strings.Join(cc.Terms, ", "))},
		{label: "模型服务类型", key: "correction_provider", help: "DeepSeek 会关闭思考并启用 JSON 输出", fType: fSelect, opts: []string{"deepseek", "openai-compatible"}, optIdx: idxOf([]string{"deepseek", "openai-compatible"}, cc.Provider)},
		{label: "模型服务地址", key: "correction_base_url", help: "OpenAI 兼容 Base URL，通常含 /v1", fType: fString, input: ti(cc.BaseURL)},
		{label: "纠错模型", key: "correction_model", help: "服务商提供的模型名；优先使用快速非思考模型", fType: fString, input: ti(cc.Model)},
		{label: "模型 API Key", key: "correction_api_key", help: "只保存于本地配置，编辑时也隐藏", fType: fString, input: ti(cc.APIKey)},
		{label: "纠错超时(ms)", key: "correction_timeout_ms", help: "500–30000；失败时保留原始识别，不自动上屏", fType: fString, input: ti(fmt.Sprint(cc.TimeoutMS))},
	}
	for i := range fs {
		if fs[i].key == "correction_api_key" {
			fs[i].input.EchoMode = textinput.EchoPassword
			fs[i].input.EchoCharacter = '•'
		}
	}
	return &Model{cfg: cfg, fields: fs, logs: make([]string, 0, 100), cursor: -1, showLogs: true}
}

func (m *Model) SetDebug(debug bool) {
	m.debug = debug
}

func idxOf(opts []string, v string) int {
	for i, o := range opts {
		if o == v {
			return i
		}
	}
	return 0
}

func (m *Model) Init() tea.Cmd { return tea.Batch(fetchDevices(), tea.EnterAltScreen, tickRefresh()) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h, m.ready = msg.Width, msg.Height, true
		for i := range m.fields {
			m.fields[i].input.Width = max(10, m.w-24)
		}
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case backendMsg:
		m.info = hotkey.ProviderInfo(msg)
	case refreshMsg:
		return m, tickRefresh()
	case devMsg:
		if msg.Error != nil {
			m.logf("设备: %s", msg.Error)
		} else {
			m.devices = msg.Devices
		}
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()

	// Editing a field
	if m.editing && m.cursor >= 0 && m.cursor < len(m.fields) {
		f := &m.fields[m.cursor]
		switch k {
		case "esc":
			m.editing = false
			f.input.Blur()
			return nil
		case "enter":
			m.editing = false
			f.input.Blur()
			m.save()
			return nil
		}
		switch f.fType {
		case fSelect:
			switch k {
			case "j", "down":
				f.optIdx++
				if f.optIdx >= len(f.opts) {
					f.optIdx = 0
				}
			case "k", "up":
				f.optIdx--
				if f.optIdx < 0 {
					f.optIdx = len(f.opts) - 1
				}
			}
		case fToggle:
			switch k {
			case " ":
				f.boolVal = !f.boolVal
			case "j", "down", "k", "up":
				m.editing = false
				m.cursor++
				if m.cursor >= len(m.fields) {
					m.cursor = 0
				}
			}
		case fString:
			var cmd tea.Cmd
			f.input, cmd = f.input.Update(msg)
			return cmd
		}
		return nil
	}

	// Navigation mode
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "s":
		m.save()
		return nil
	case "e", "i", "enter":
		m.editing = true
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.fields[m.cursor].fType == fString {
			m.fields[m.cursor].input.Focus()
		}
	case "j", "down":
		if m.cursor < 0 {
			m.cursor = 0
		} else {
			m.cursor++
			if m.cursor >= len(m.fields) {
				m.cursor = len(m.fields) - 1
			}
		}
	case "k", "up":
		if m.cursor < 0 {
			m.cursor = 0
		} else {
			m.cursor--
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
	case "l":
		if m.showLogs {
			m.logExpanded = !m.logExpanded
		}
	case "h":
		m.helpVisible = !m.helpVisible
	}
	return nil
}
func (m *Model) save() {
	next := *m.cfg
	vc := &next.Voice
	for _, f := range m.fields {
		switch f.key {
		case "enabled":
			vc.Enabled = f.boolVal
		case "push_to_talk":
			vc.PushToTalk = f.input.Value()
		case "mode":
			vc.Mode = f.opts[f.optIdx]
		case "app_key":
			vc.AppKey = f.input.Value()
		case "access_key":
			vc.AccessKey = f.input.Value()
		case "auto_submit":
			vc.AutoSubmit = f.boolVal
		case "stop_delay_ms":
			fmt.Sscanf(f.input.Value(), "%d", &vc.StopDelayMs)
		case "hotwords":
			vc.Hotwords = splitList(f.input.Value())
		case "boosting_table_id":
			vc.BoostingTableID = strings.TrimSpace(f.input.Value())
		case "correction_enabled":
			next.Correction.Enabled = f.boolVal
		case "correction_terms":
			next.Correction.Terms = splitList(f.input.Value())
		case "correction_provider":
			next.Correction.Provider = f.opts[f.optIdx]
		case "correction_base_url":
			next.Correction.BaseURL = strings.TrimSpace(f.input.Value())
		case "correction_model":
			next.Correction.Model = strings.TrimSpace(f.input.Value())
		case "correction_api_key":
			next.Correction.APIKey = strings.TrimSpace(f.input.Value())
		case "correction_timeout_ms":
			if _, err := fmt.Sscanf(f.input.Value(), "%d", &next.Correction.TimeoutMS); err != nil {
				m.logf("❌ 纠错超时必须是毫秒整数")
				return
			}
		}
	}
	if err := correction.Validate(next.Correction); err != nil {
		m.logf("❌ %s", err)
		return
	}
	combo, err := config.ParseHotkey(vc.PushToTalk)
	if err != nil {
		m.logf("❌ 热键格式错误: %s", err)
		m.restorePushToTalkField()
		return
	}
	if combo.Key.IsTextKey() {
		m.logf("❌ 热键不支持普通字符键: %s", combo)
		m.logf("   请使用 Alt+Super、F9、Alt+F8、Ctrl+Alt+Tab 等全局快捷键")
		m.restorePushToTalkField()
		return
	}
	m.cfg = &next
	if err := config.Save(m.cfg); err != nil {
		m.logf("保存失败: %s", err)
	} else {
		m.logf("✅ 配置已保存到 %s", m.cfg.Path())
	}
	m.logf("  push_to_talk=%s", vc.PushToTalk)
	if m.OnSave != nil {
		if err := m.OnSave(m.cfg); err != nil {
			m.logf("❌ 热键注册失败: %s", err)
		}
	}
}

func (m *Model) restorePushToTalkField() {
	value := m.cfg.Voice.PushToTalk
	if !validVoiceHotkeyString(value) {
		value = config.Default().Voice.PushToTalk
	}
	for i := range m.fields {
		if m.fields[i].key == "push_to_talk" {
			m.fields[i].input.SetValue(value)
			break
		}
	}
	m.logf("↩️  热键已恢复为 %s", value)
}

func validVoiceHotkeyString(value string) bool {
	combo, err := config.ParseHotkey(value)
	return err == nil && !combo.Key.IsTextKey()
}

func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}
func (m *Model) View() string {
	if !m.ready {
		return "loading..."
	}
	var b strings.Builder
	b.WriteString(tStyle.Render("🎙️ 🗣️ Just Talk") + "\n")
	b.WriteString(vStyle.Render("减少用键盘的次数，改用口喷吧。") + "\n")
	b.WriteString(m.renderVoiceStats() + "\n\n")
	rows := min(len(m.fields), max(3, min(8, m.h-16)))
	first := max(0, min(m.cursor-rows+1, len(m.fields)-rows))
	b.WriteString(lStyle.Render(fmt.Sprintf("── 配置 %d–%d/%d (e 编辑, s 保存, h 帮助, j/k 导航) ──", first+1, first+rows, len(m.fields))) + "\n")
	for i, f := range m.fields {
		if i < first || i >= first+rows {
			continue
		}
		marker := "  "
		if i == m.cursor {
			if m.editing {
				marker = aStyle.Render("▶ ")
			} else {
				marker = aStyle.Render("▸ ")
			}
		}
		line := marker + lStyle.Render(f.label+": ") + m.renderField(i, f)
		if m.helpVisible && f.help != "" {
			line += " " + dStyle.Render("("+f.help+")")
		}
		b.WriteString(ansi.Truncate(line, max(1, m.w), "…") + "\n")
	}
	b.WriteString("\n" + lStyle.Render("── 录音状态 ──") + "\n")
	b.WriteString("  " + ansi.Truncate(m.renderVoiceStatus(), max(1, m.w-2), "…") + "\n")
	if m.showLogs {
		b.WriteString("\n" + lStyle.Render("── 日志 (l 展开) ──") + "\n")
		maxLogs := max(1, min(5, m.h-strings.Count(b.String(), "\n")-2))
		if m.logExpanded {
			maxLogs = 100
		}
		logs := append(append([]string(nil), m.logs...), voice.TUILogs()...)
		if len(logs) > maxLogs {
			logs = logs[len(logs)-maxLogs:]
		}
		for _, line := range logs {
			b.WriteString("  " + dStyle.Render(ansi.Truncate(line, max(1, m.w-2), "…")) + "\n")
		}
	}
	b.WriteString(hStyle.Render("  j/k 导航 | e 编辑 | h 帮助 | esc 退出编辑 | s 保存 | q 退出"))
	return b.String()
}

func (m *Model) renderVoiceStats() string {
	stats := voice.TUIStats()
	cpm := 0.0
	if stats.AudioDuration > 0 {
		cpm = float64(stats.Chars) / stats.AudioDuration.Minutes()
	}
	lastCPM := 0.0
	if stats.LastAudioDuration > 0 {
		lastCPM = float64(stats.LastTextChars) / stats.LastAudioDuration.Minutes()
	}
	last := dStyle.Render("最近速度 ") + wStyle.Render("0") + dStyle.Render(" 字/分钟")
	if stats.LastAudioDuration > 0 {
		last = dStyle.Render("最近速度 ") + wStyle.Render(fmt.Sprintf("%.0f", lastCPM)) + dStyle.Render(" 字/分钟")
	}
	return strings.Join([]string{
		dStyle.Render("历史 ") + aStyle.Render(fmt.Sprintf("%d", stats.Sessions)) + dStyle.Render(" 次"),
		dStyle.Render("总字数 ") + aStyle.Render(fmt.Sprintf("%d", stats.Chars)),
		dStyle.Render("平均速度 ") + wStyle.Render(fmt.Sprintf("%.0f", cpm)) + dStyle.Render(" 字/分钟"),
		last,
	}, "  |  ")
}

func (m *Model) renderVoiceStatus() string {
	status := voice.TUIStatus()
	label := "待机"
	style := dStyle
	switch status.State {
	case "connecting":
		label, style = "连接中", wStyle
	case "recording":
		label, style = "录音中", aStyle
	case "stopping_delayed":
		label, style = "延迟停止", wStyle
	case "stopping":
		label, style = "停止中", wStyle
	case "correcting":
		label, style = "纠错中", wStyle
	case "error":
		label, style = "错误", eStyle
	}
	parts := []string{style.Render(label)}
	if status.Detail != "" {
		parts = append(parts, vStyle.Render(status.Detail))
	}
	if !status.StopAt.IsZero() {
		remaining := time.Until(status.StopAt)
		if remaining < 0 {
			remaining = 0
		}
		parts = append(parts, fmt.Sprintf("剩余 %dms", remaining.Milliseconds()))
	}
	if status.State == "error" && !status.ErrorUntil.IsZero() {
		remaining := time.Until(status.ErrorUntil)
		if remaining < 0 {
			remaining = 0
		}
		parts = append(parts, fmt.Sprintf("保留 %ds", int(remaining.Seconds())))
	}
	if status.PendingFinishes > 0 {
		parts = append(parts, fmt.Sprintf("后台收尾 %d", status.PendingFinishes))
	}
	if status.SessionID > 0 {
		parts = append(parts, dStyle.Render(fmt.Sprintf("#%d", status.SessionID)))
	}
	if !m.debug {
		return strings.Join(parts, "  ")
	}
	if !status.UpdatedAt.IsZero() {
		age := time.Since(status.UpdatedAt)
		if age < 0 {
			age = 0
		}
		parts = append(parts, dStyle.Render(fmt.Sprintf("更新 %dms 前", age.Milliseconds())))
	}
	if !status.LastHotkeyAt.IsZero() {
		age := time.Since(status.LastHotkeyAt)
		if age < 0 {
			age = 0
		}
		parts = append(parts, dStyle.Render(fmt.Sprintf("热键 %s %dms 前", status.LastHotkeyType, age.Milliseconds())))
	}
	if !status.LastHandledAt.IsZero() {
		age := time.Since(status.LastHandledAt)
		if age < 0 {
			age = 0
		}
		parts = append(parts, dStyle.Render(fmt.Sprintf("处理 %s %dms 前", status.LastHandledType, age.Milliseconds())))
	}
	if status.QueuedHotkeys > 0 || status.HandledHotkeys > 0 {
		parts = append(parts, dStyle.Render(fmt.Sprintf("入队/处理 %d/%d q=%d", status.QueuedHotkeys, status.HandledHotkeys, status.EventQueueLen)))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) renderField(i int, f field) string {
	editing := m.editing && m.cursor == i
	switch f.fType {
	case fString:
		v := f.input.Value()
		if f.key == "correction_api_key" && !editing && v != "" {
			v = "••••••••"
		}
		if f.key == "access_key" && !editing && len(v) > 8 {
			v = v[:8] + "***"
		}
		if editing {
			return f.input.View()
		}
		return vStyle.Render(v)
	case fToggle:
		if f.boolVal {
			return aStyle.Render("● 开") + "  " + dStyle.Render("(空格)")
		}
		return dStyle.Render("○ 关  (空格)")
	case fSelect:
		v := f.opts[f.optIdx]
		if editing {
			return aStyle.Render("[" + v + " ▲▼]")
		}
		return vStyle.Render(v)
	}
	return ""
}

func (m *Model) logf(format string, args ...interface{}) {
	if !m.showLogs {
		return
	}
	m.logs = append(m.logs, fmt.Sprintf(format, args...))
}

func SetProviderInfo(info hotkey.ProviderInfo) tea.Cmd {
	return func() tea.Msg { return backendMsg(info) }
}
func fetchDevices() tea.Cmd {
	return func() tea.Msg {
		devices, err := voice.ListDevices()
		if err != nil {
			return devMsg{Error: err}
		}
		return devMsg{Devices: devices}
	}
}

type refreshMsg struct{}

func tickRefresh() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return refreshMsg{} })
}

// Handle refreshMsg in Update
