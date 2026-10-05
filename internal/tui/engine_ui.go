// Copyright (C) 2026 just-talk-go contributors
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk.

// Engine picker + per-engine config modals for the TUI.
//
// Design: single flat main form stays untouched; pressing enter on the
// "引擎"/"引擎配置" action fields opens a centered modal overlay rendered on
// top of the normal view. Engine credentials live under
// voice.engine_configs[engine] so switching engines never clobbers keys.

package tui

import (
	"strings"

	"github.com/c/just-talk-go/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

type overlayState int

const (
	overlayNone overlayState = iota
	overlayPicker
	overlayEngineConfig
)

// engineMeta describes one selectable engine for the picker UI.
// Order here is the picker order: streaming engines first.
type engineMeta struct {
	name      string   // registry type
	tag       string   // short capability badge
	blurb     string   // one-line pitch shown under the name
	doc       string   // doc URL opened with `o`
	streaming bool     // live partials → real-time captions
	required  []string // config keys that make the engine usable
}

var engineMetas = []engineMeta{
	{
		name:      "doubao-stream",
		tag:       "流式",
		blurb:     "豆包大模型双向流式 ASR，边说边出字 + 二遍校正，支持热词",
		doc:       "https://www.volcengine.com/docs/6561/1395867",
		streaming: true,
		// Available() accepts either api_key (new console) or app_id+access_key
		// (legacy); checked specially in engineConfigured.
		required: nil,
	},
	{
		name:      "dashscope",
		tag:       "流式",
		blurb:     "阿里千问 Fun-ASR 实时流式，边录边出字；8k 模型自动降采样",
		doc:       "https://www.alibabacloud.com/help/en/model-studio/fun-asr-realtime-websocket-api",
		streaming: true,
		required:  []string{"api_key"},
	},
	{
		name:     "openai",
		tag:      "整段",
		blurb:    "OpenAI 兼容 Whisper 接口，整段识别；可指向本地/第三方端点",
		doc:      "https://platform.openai.com/docs/api-reference/audio",
		required: []string{"api_key"},
	},
	{
		name:     "auralwise",
		tag:      "整段",
		blurb:    "AuralWise 异步任务转写，整段识别后上屏",
		doc:      "https://auralwise.cn/",
		required: []string{"api_key"},
	},
	{
		name:     "mimo-asr",
		tag:      "整段",
		blurb:    "小米 MiMo 语音识别，整段识别",
		doc:      "https://platform.xiaomimimo.com/",
		required: []string{"api_key"},
	},
}

func metaFor(engine string) engineMeta {
	for _, m := range engineMetas {
		if m.name == engine {
			return m
		}
	}
	return engineMeta{name: engine, tag: "整段", blurb: "自定义引擎"}
}

// ecFieldSpec declares one field in the per-engine config modal.
type ecFieldSpec struct {
	label    string
	key      string // stored under engine_configs[engine]
	help     string
	fType    fieldType
	opts     []string // fSelect options
	required bool
	secret   bool // mask when not editing
}

func engineFieldSpecs(engine string) []ecFieldSpec {
	switch engine {
	case "doubao-stream":
		return []ecFieldSpec{
			{label: "API Key", key: "api_key", help: "新版控制台 X-Api-Key；填了就忽略下方旧版凭据", secret: true},
			{label: "App ID", key: "app_id", help: "旧版控制台 App ID（X-Api-App-Key）"},
			{label: "Access Key", key: "access_key", help: "旧版 Access Token（X-Api-Access-Key）", required: true, secret: true},
			{label: "模型版本", key: "model_version", help: "1.0 = bigasr（旧资源），2.0 = seedasr（新控制台）", fType: fSelect, opts: []string{"1.0", "2.0"}},
			{label: "流式计费", key: "stream_billing", help: "duration 按时长 / concurrent 按并发，对照套餐选择", fType: fSelect, opts: []string{"duration", "concurrent"}},
			{label: "Resource ID", key: "resource_id", help: "留空则按模型版本+计费推导；除非控制台另有资源 ID 一般不填"},
			{label: "中英混识别", key: "enable_lid", help: "中英混说/方言时开启", fType: fSelect, opts: []string{"false", "true"}},
			{label: "Base Host", key: "base_host", help: "默认 openspeech.bytedance.com；仅自建代理/网关时改"},
		}
	case "dashscope":
		return []ecFieldSpec{
			{label: "API Key", key: "api_key", help: "必填，DashScope 控制台创建（sk-…）", required: true, secret: true},
			{label: "模型", key: "model", help: "默认 fun-asr-realtime；8k 型号（名字含 8k）自动降到 8000 Hz"},
			{label: "采样率", key: "sample_rate", help: "16000 通用；8000 仅 8k 模型", fType: fSelect, opts: []string{"16000", "8000"}},
		}
	case "openai":
		return []ecFieldSpec{
			{label: "API Key", key: "api_key", help: "必填，Bearer token", required: true, secret: true},
			{label: "Base URL", key: "base_url", help: "默认 https://api.openai.com/v1，可指本地 whisper.cpp server"},
			{label: "模型", key: "model", help: "默认 whisper-1"},
		}
	case "auralwise":
		return []ecFieldSpec{
			{label: "API Key", key: "api_key", help: "必填，auralwise.cn 账号设置里创建（asr_…）", required: true, secret: true},
			{label: "Base URL", key: "base_url", help: "默认 https://api.auralwise.cn/v1"},
			{label: "Optimize", key: "optimize", help: "auto 按语言自动 / true 省成本 / false 高质量词级时间戳", fType: fSelect, opts: []string{"auto", "true", "false"}},
		}
	case "mimo-asr":
		return []ecFieldSpec{
			{label: "API Key", key: "api_key", help: "必填，小米 MiMo 控制台", required: true, secret: true},
			{label: "Base URL", key: "base_url", help: "默认 https://api.xiaomimimo.com/v1"},
			{label: "模型", key: "model", help: "默认 mimo-v2.5-asr"},
		}
	}
	return nil
}

// engineConfigured reports whether every required key of the engine is filled.
// doubao legacy flat fields count as fallbacks, matching asrcfg.ConfigMap.
func engineConfigured(vc config.VoiceConfig, engine string) bool {
	meta := metaFor(engine)
	if engine == "doubao-stream" {
		ec := vc.EngineConfigFor(engine)
		if engineConfigValue(vc, engine, ec, "api_key") != "" {
			return true
		}
		return engineConfigValue(vc, engine, ec, "app_id") != "" &&
			engineConfigValue(vc, engine, ec, "access_key") != ""
	}
	if len(meta.required) == 0 {
		return true
	}
	ec := vc.EngineConfigFor(engine)
	for _, k := range meta.required {
		if engineConfigValue(vc, engine, ec, k) == "" {
			return false
		}
	}
	return true
}

// engineConfigValue resolves a config key: engine_configs value first, then
// doubao legacy flat fields (so existing installs keep working in the UI).
func engineConfigValue(vc config.VoiceConfig, engine string, ec map[string]string, key string) string {
	if v := strings.TrimSpace(ec[key]); v != "" {
		return v
	}
	if engine != "doubao-stream" {
		return ""
	}
	switch key {
	case "app_id":
		return vc.AppKey
	case "access_key":
		return vc.AccessKey
	case "resource_id":
		return vc.ResourceID
	}
	return ""
}

// ---- modal open/close ----

func (m *Model) openEnginePicker() {
	m.overlay = overlayPicker
	m.pickerIdx = 0
	current := m.cfg.Voice.Engine
	if current == "" {
		current = "doubao-stream"
	}
	for i, meta := range engineMetas {
		if meta.name == current {
			m.pickerIdx = i
			break
		}
	}
}

func (m *Model) openEngineConfig() {
	engine := m.currentEngine()
	vc := m.cfg.Voice
	ec := vc.EngineConfigFor(engine)
	ti := func(v string) textinput.Model {
		t := textinput.New()
		t.SetValue(v)
		t.Cursor.Blink = false
		return t
	}
	specs := engineFieldSpecs(engine)
	fs := make([]field, 0, len(specs))
	for _, spec := range specs {
		f := field{label: spec.label, key: spec.key, help: spec.help, fType: spec.fType, required: spec.required, secret: spec.secret}
		v := engineConfigValue(vc, engine, ec, spec.key)
		switch spec.fType {
		case fSelect:
			f.opts = spec.opts
			f.optIdx = idxOf(spec.opts, v)
		default:
			f.input = ti(v)
		}
		fs = append(fs, f)
	}
	m.ecFields = fs
	m.ecCursor = 0
	m.ecEditing = false
	m.overlay = overlayEngineConfig
}

func (m *Model) currentEngine() string {
	engine := strings.TrimSpace(m.cfg.Voice.Engine)
	if engine == "" {
		return "doubao-stream"
	}
	return engine
}

func (m *Model) closeOverlay() {
	m.overlay = overlayNone
	m.ecEditing = false
	for i := range m.ecFields {
		m.ecFields[i].input.Blur()
	}
}

// ---- modal key handling ----

func (m *Model) handleOverlayKey(msg tea.KeyMsg) bool {
	switch m.overlay {
	case overlayPicker:
		m.handlePickerKey(msg.String())
		return true
	case overlayEngineConfig:
		m.handleConfigKey(msg)
		return true
	}
	return false
}

func (m *Model) handlePickerKey(k string) {
	switch k {
	case "esc", "q":
		m.closeOverlay()
	case "j", "down":
		m.pickerIdx++
		if m.pickerIdx >= len(engineMetas) {
			m.pickerIdx = 0
		}
	case "k", "up":
		m.pickerIdx--
		if m.pickerIdx < 0 {
			m.pickerIdx = len(engineMetas) - 1
		}
	case "o":
		openURL(engineMetas[m.pickerIdx].doc)
	case "enter", "e", "i":
		m.cfg.Voice.Engine = engineMetas[m.pickerIdx].name
		m.openEngineConfig()
	}
}

func (m *Model) handleConfigKey(msg tea.KeyMsg) {
	k := msg.String()
	if m.ecEditing && m.ecCursor >= 0 && m.ecCursor < len(m.ecFields) {
		f := &m.ecFields[m.ecCursor]
		switch k {
		case "esc":
			m.ecEditing = false
			f.input.Blur()
			return
		case "enter":
			m.ecEditing = false
			f.input.Blur()
			m.commitEngineConfig(f)
			return
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
		case fString:
			f.input, _ = f.input.Update(msg)
			return
		}
		return
	}

	switch k {
	case "esc", "q":
		m.closeOverlay()
	case "o":
		openURL(metaFor(m.currentEngine()).doc)
	case "s":
		m.save()
	case "j", "down":
		m.ecCursor++
		if m.ecCursor >= len(m.ecFields) {
			m.ecCursor = 0
		}
	case "k", "up":
		m.ecCursor--
		if m.ecCursor < 0 {
			m.ecCursor = len(m.ecFields) - 1
		}
	case "e", "i", "enter":
		if m.ecCursor >= 0 && m.ecCursor < len(m.ecFields) {
			m.ecEditing = true
			f := &m.ecFields[m.ecCursor]
			if f.fType == fString {
				f.input.Focus()
			}
		}
	}
}

// commitEngineConfig writes one edited field into cfg.Voice.EngineConfigs.
func (m *Model) commitEngineConfig(f *field) {
	engine := m.currentEngine()
	ec := m.cfg.Voice.EngineConfigFor(engine)
	if ec == nil {
		ec = map[string]string{}
	}
	switch f.fType {
	case fSelect:
		v := f.opts[f.optIdx]
		if v == "" {
			delete(ec, f.key)
		} else {
			ec[f.key] = v
		}
	default:
		v := strings.TrimSpace(f.input.Value())
		if v == "" {
			delete(ec, f.key)
		} else {
			ec[f.key] = v
		}
	}
	m.cfg.Voice.SetEngineConfig(engine, ec)
}

// ---- modal rendering ----

var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			MaxWidth(84).
			Padding(0, 1)
	modalTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

func (m *Model) renderPicker() string {
	var b strings.Builder
	b.WriteString(modalTitle.Render("选择识别引擎") + "\n")
	b.WriteString(dStyle.Render("回车选定并进入引擎配置，o 打开文档，esc 返回") + "\n\n")
	current := m.currentEngine()
	for i, meta := range engineMetas {
		marker := "  "
		if i == m.pickerIdx {
			marker = aStyle.Render("▸ ")
		}
		chosen := "  "
		if meta.name == current {
			chosen = aStyle.Render("● ")
		}
		var tag string
		if meta.streaming {
			tag = aStyle.Render("[流式]")
		} else {
			tag = dStyle.Render("[整段]")
		}
		status := dStyle.Render("未配置")
		if engineConfigured(m.cfg.Voice, meta.name) {
			status = aStyle.Render("已配置")
		}
		b.WriteString(marker + chosen + vStyle.Render(meta.name) + " " + tag + " " + status + "\n")
		b.WriteString("      " + dStyle.Render(meta.blurb) + "\n")
	}
	return modalStyle.Render(b.String())
}

func (m *Model) renderEngineConfig() string {
	engine := m.currentEngine()
	meta := metaFor(engine)
	var b strings.Builder
	b.WriteString(modalTitle.Render("引擎配置 · "+meta.name) + "  ")
	if meta.streaming {
		b.WriteString(aStyle.Render("[流式]"))
	} else {
		b.WriteString(dStyle.Render("[整段]"))
	}
	b.WriteString("\n" + dStyle.Render(meta.blurb) + "\n\n")
	for i, f := range m.ecFields {
		marker := "  "
		if i == m.ecCursor {
			if m.ecEditing {
				marker = aStyle.Render("▶ ")
			} else {
				marker = aStyle.Render("▸ ")
			}
		}
		label := f.label
		if f.required {
			label += "*"
		}
		line := marker + lStyle.Render(label+": ") + m.renderECField(i, f)
		if f.help != "" {
			line += " " + dStyle.Render("("+f.help+")")
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + hStyle.Render("  j/k 导航 | e 编辑 | 回车提交 | s 保存 | o 文档 | esc 返回"))
	return modalStyle.Render(squeezeWidth(b.String(), 72))
}

// squeezeWidth truncates every line of s to maxCells display cells so the
// surrounding border never exceeds the modal cap. ANSI-aware via cellSlice.
func squeezeWidth(s string, maxCells int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > maxCells {
			lines[i] = cellSlice(l, 0, maxCells)
		}
	}
	return strings.Join(lines, "\n")
}

// renderECField renders one engine-config field; secrets mask when not editing.
func (m *Model) renderECField(i int, f field) string {
	editing := m.ecEditing && m.ecCursor == i
	switch f.fType {
	case fString:
		v := f.input.Value()
		if f.secret && !editing && len(v) > 8 {
			v = v[:8] + "***"
		}
		if editing {
			return f.input.View()
		}
		if v == "" {
			return dStyle.Render("(空)")
		}
		return vStyle.Render(v)
	case fSelect:
		v := f.opts[f.optIdx]
		if v == "" {
			v = "(默认)"
		}
		if editing {
			return aStyle.Render("[" + v + " ▲▼]")
		}
		return vStyle.Render(v)
	}
	return ""
}

// ---- compositing ----

// compositeOver draws box centered on base, replacing those cells.
// ANSI-aware: cellSlice walks escape sequences without counting them.
func compositeOver(base, box string, w, h int) string {
	boxLines := strings.Split(box, "\n")
	const maxBoxW = 80
	for i, l := range boxLines {
		if lipgloss.Width(l) > maxBoxW {
			boxLines[i] = cellSlice(l, 0, maxBoxW)
		}
	}
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}
	boxW := 0
	for _, l := range boxLines {
		if lw := lipgloss.Width(l); lw > boxW {
			boxW = lw
		}
	}
	boxH := len(boxLines)
	if boxW >= w || boxH >= h {
		return box // terminal smaller than modal: just show modal
	}
	top := (h - boxH) / 2
	left := (w - boxW) / 2
	for i, l := range boxLines {
		row := top + i
		lw := lipgloss.Width(l)
		prefix := cellSlice(baseLines[row], 0, left)
		if lp := lipgloss.Width(prefix); lp < left {
			prefix += strings.Repeat(" ", left-lp)
		}
		baseLines[row] = prefix + l + cellSlice(baseLines[row], left+lw, w)
	}
	return strings.Join(baseLines, "\n")
}

// cellSlice returns the substring of s covering display cells [start, end).
// ANSI escape sequences pass through without consuming cells; a trailing
// reset is appended so the following text is not contaminated by open styles.
func cellSlice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	var state int // 0 normal, 1 esc, 2 csi, 3 osc
	col := 0
	inRange := false
	for _, r := range s {
		switch state {
		case 0:
			if r == 0x1b {
				state = 1
				if inRange {
					b.WriteRune(r)
				}
				continue
			}
			w := uniseg.StringWidth(string(r))
			if col+w > start && col < end {
				inRange = true
			}
			if inRange && col < end {
				b.WriteRune(r)
			}
			col += w
			if col >= end {
				inRange = false
			}
		case 1:
			if inRange {
				b.WriteRune(r)
			}
			switch r {
			case '[':
				state = 2
			case ']':
				state = 3
			default:
				state = 0
			}
		case 2: // CSI: ends on a letter
			if inRange {
				b.WriteRune(r)
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				state = 0
			}
		case 3: // OSC: ends on BEL or ESC-backslash
			if inRange {
				b.WriteRune(r)
			}
			if r == 0x07 {
				state = 0
			}
			// ESC-terminated OSC is rare in TUI output; BEL suffices here.
		}
	}
	if b.Len() > 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}
