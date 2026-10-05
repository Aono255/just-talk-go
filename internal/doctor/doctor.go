package doctor

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/c/just-talk-go/config"
	"github.com/c/just-talk-go/internal/asrcfg"
	"github.com/c/just-talk-go/internal/speech"
)

type Severity int

const (
	Required Severity = iota
	Warning
)

type Check struct {
	Name     string
	OK       bool
	Severity Severity
	Detail   string
	Notes    []string
	Fix      string
}

type Report struct {
	Platform string
	Backend  string
	Info     []string
	Checks   []Check
}

func Run(cfg *config.Config, backend string) Report {
	return runPlatform(cfg, backend)
}

func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Severity == Required && !check.OK {
			return false
		}
	}
	return true
}

func (r Report) Print(w io.Writer) {
	fmt.Fprintln(w, "Just Talk 环境检查")
	fmt.Fprintf(w, "平台：%s", platformName(fallback(r.Platform, "unknown")))
	if r.Backend != "" {
		fmt.Fprintf(w, " / %s", r.Backend)
	}
	fmt.Fprintln(w)
	for _, line := range r.Info {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(w, line)
		}
	}
	fmt.Fprintln(w)

	for _, check := range r.Checks {
		mark := "✓"
		if !check.OK {
			if check.Severity == Warning {
				mark = "!"
			} else {
				mark = "✗"
			}
		}
		fmt.Fprintf(w, "%s %s", mark, check.Name)
		if check.Detail != "" {
			fmt.Fprintf(w, "：%s", check.Detail)
		}
		fmt.Fprintln(w)
		for _, note := range check.Notes {
			if strings.TrimSpace(note) != "" {
				fmt.Fprintf(w, "  %s\n", note)
			}
		}
		if !check.OK && check.Fix != "" {
			fmt.Fprintf(w, "  处理：%s\n", check.Fix)
		}
	}

	if r.Healthy() {
		fmt.Fprintln(w, "\n结果：环境正常")
	} else {
		fmt.Fprintln(w, "\n结果：需要处理上面的项目后再启动 Just Talk。")
	}
}

func fallback(s, v string) string {
	if strings.TrimSpace(s) == "" {
		return v
	}
	return s
}

func platformName(s string) string {
	switch strings.ToLower(s) {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return s
	}
}

// asrEngineCheck builds and probes the configured engine.
func asrEngineCheck(cfg *config.Config, cfgPath string) Check {
	eng, err := asrcfg.Build(cfg.Voice)
	if err != nil {
		return Check{
			Name: "ASR 配置", OK: false, Severity: Warning,
			Detail: "引擎 " + asrcfg.EngineType(cfg.Voice) + "：" + err.Error(),
			Fix:    "在 " + cfgPath + " 的 [voice].engine_configs 中填写该引擎所需凭据。",
		}
	}
	status := speech.Probe(context.Background(), eng)
	if !status.Available {
		return Check{
			Name: "ASR 配置", OK: false, Severity: Warning,
			Detail: status.Engine + "：" + status.Detail,
			Fix:    "在 " + cfgPath + " 的 [voice].engine_configs 中填写该引擎所需凭据。",
		}
	}
	return Check{Name: "ASR 配置", OK: true, Severity: Warning, Detail: status.Engine + "：" + status.Detail}
}
