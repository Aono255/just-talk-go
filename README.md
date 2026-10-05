<p align="center">
  <img src="docs/assets/brand/icon-d.svg" alt="Just Talk" width="96">
</p>

# Just Talk

[中文](README.md) · [English](README.en.md)

[![Release](https://img.shields.io/github/v/release/wakaka6/just-talk-go?label=release)](https://github.com/wakaka6/just-talk-go/releases)
[![Release workflow](https://github.com/wakaka6/just-talk-go/actions/workflows/release.yml/badge.svg)](https://github.com/wakaka6/just-talk-go/actions/workflows/release.yml)
[![Go version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](go.mod)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-wakaka6%2Ftap-FBB040?logo=homebrew&logoColor=white)](https://github.com/wakaka6/homebrew-tap)

减少用键盘的次数，改用口喷吧。

Just Talk 是一个面向桌面环境的语音输入工具。它通过全局快捷键录音，把语音流式发送到豆包大模型 ASR，识别结果自动复制到剪贴板，或直接上屏到当前输入框——适合写代码、聊天、记笔记和处理长文本输入。

![Just Talk TUI](docs/screenshot-tui.png)

## macOS 刘海浮层

在 macOS 上，录音提示直接从刘海展开：实时识别文字随语音流式显现（打字机式逐段展开），旁边同步显示麦克风音量波形；识别完成后停留展示最终文字与“已复制 / 已上屏”状态。

![macOS 实机刘海浮层：录音中的实时识别文字与音量波形](docs/assets/notch-live.gif)

![macOS 实机刘海浮层：识别完成后的已复制状态](docs/assets/notch-live-final.png)

屏幕没有刘海（外接显示器或无刘海机型）时自动回退为顶部居中的状态胶囊。通过 `position = "notch"` 或 TUI 的“提示位置”启用，行为细节见 [配置](#配置)。

## 功能

- 全局快捷键录音，支持 `toggle`（按一下开始、再按一下停止）和 `hold`（按住说话）两种模式。
- 语音热键限定为适合作为全局快捷键的按键：纯修饰键组合、功能键、Tab、CapsLock、方向键和导航键等；不支持字母、数字、标点、空格等普通文本键。
- 豆包大模型流式 ASR（双向流优化版 + 二遍识别），支持热词增强。
- 识别结果自动复制到剪贴板，可选自动上屏到焦点输入框。
- 录音状态浮层：macOS 默认从刘海展开，显示实时识别文字、麦克风音量和完成结果；Linux（Wayland / X11）和 Windows 使用置顶状态胶囊。
- TUI 配置界面：热键、模式、自动上屏、停止延迟、热词、提示位置等，保存后即时生效，无需重启。
- 录音历史统计：累计次数、总字数、平均速度和最近速度。
- 录音中 `Esc` 取消，可重试错误时 `R` 重试。

## 平台支持

| 平台 | 状态 | 说明 |
| --- | --- | --- |
| Linux Wayland | 已支持 | 已验证 Sway / wlroots；快捷键基于 evdev，需要 `input` 组权限；剪贴板/上屏依赖 `wl-clipboard` + `wtype` 或 `/dev/uinput` |
| Linux X11 | 已支持 | 原生 X11 全局热键与浮层，XTest 自动上屏 |
| macOS | 已支持 | CGEventTap 热键、CoreAudio 录音、NSPasteboard 剪贴板、AppKit NSPanel 浮层；需要辅助功能和麦克风权限 |
| Windows 10/11 | 已支持 | 全局按键轮询 + 低级键盘钩子边沿回退、WinMM 录音、Unicode 剪贴板、SendInput 上屏、Win32 置顶浮层 |

## 安装

### Homebrew（macOS / Linux）

```bash
brew install wakaka6/tap/just-talk
```

来源：[wakaka6/homebrew-tap](https://github.com/wakaka6/homebrew-tap)。Formula 覆盖 macOS（Apple Silicon / Intel）和 Linux（amd64 / arm64）。

### 预编译二进制

<details>
<summary>展开查看各平台归档下载与校验说明</summary>

从 [GitHub Releases](https://github.com/wakaka6/just-talk-go/releases) 下载对应平台的归档，解压后将 `just-talk` 放入 `PATH`（如 `~/.local/bin`）。Windows 提供 `.zip`，其他平台为 `.tar.gz`，可用归档中的 `SHA256SUMS.txt` 校验。

当前提供：Linux amd64 / arm64、macOS Intel / Apple Silicon、Windows amd64 / arm64。

</details>

### 源码构建

<details>
<summary>展开查看构建依赖与安装步骤</summary>

Just Talk 依赖平台原生能力：Linux 和 macOS 需要启用 cgo；Windows 使用纯 Go 调用 Win32，不需要 cgo。

Linux 构建依赖：

```bash
# Arch Linux
sudo pacman -S --needed go gcc libx11 libxtst libxext wayland

# Debian / Ubuntu
sudo apt install golang-go build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libwayland-dev
```

macOS 构建依赖 Apple Command Line Tools（提供 clang 和 macOS SDK，不需要完整 Xcode）：

```bash
xcode-select --install
```

Windows 只需要 Go 1.25 或更高版本，不需要 ffmpeg、SoX 或 C 编译器：

```powershell
winget install --id GoLang.Go --exact
```

构建：

```bash
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
# Windows PowerShell：
# go build -o build\just-talk.exe .\cmd\just-talk
```

安装到用户目录（Linux/macOS 为 `~/.local/bin`，Windows 为 `%LOCALAPPDATA%\Programs\Just Talk`）：

```bash
build/just-talk --install
# 或
make install
```

如提示安装目录不在 `PATH` 中，按命令输出的说明添加即可。

</details>

## 快速开始

1. 获取豆包大模型语音识别凭证（App Key / Access Key）。
2. 运行 `just-talk --doctor` 检查麦克风、剪贴板工具和权限。
3. 运行 `just-talk` 打开 TUI，选中“引擎”回车选择渠道（默认豆包流式），在弹出的引擎配置中填入凭证，按 `s` 保存。
4. 按 `push_to_talk` 热键（默认 `Alt+Super`；macOS 为 `Option+Command`）开始/停止录音。识别结果会复制到剪贴板，并在开启 `auto_submit` 时上屏到当前输入框。

### 权限

<details>
<summary>展开查看各平台权限要求</summary>

- **Linux Wayland**：全局热键通过 evdev 读取 `/dev/input/event*`，需要把当前用户加入 `input` 组后重新登录：

  ```bash
  sudo usermod -aG input $USER
  ```

  剪贴板需要 `wl-clipboard`；自动上屏需要 `wtype`，或授予 `/dev/uinput` 写权限（KDE Plasma 推荐 uinput，`--doctor` 会打印完整配置步骤）。

- **macOS**：在“系统设置 → 隐私与安全性”中为启动 Just Talk 的终端应用（Terminal、iTerm2 等）授予 **辅助功能** 和 **麦克风** 权限。不需要 `.app` 签名或完整 Xcode。
- **Windows**：无需管理员权限。若麦克风不可用，在“设置 → 隐私和安全性 → 麦克风”中允许桌面应用访问麦克风。

</details>

## 快捷键

### 语音热键

| 按键 | 行为 |
| --- | --- |
| 配置的 `push_to_talk` | `toggle` 模式下开始/停止录音；`hold` 模式下按住说话 |
| `Esc` | 取消当前录音或错误提示 |
| `R` | 出现可重试错误时重新识别 |

语音热键只支持适合作为全局快捷键的按键：

- 纯修饰键组合：`Alt+Super`、`Ctrl+Alt+Shift`、`Option+Command` 等。
- `Fn`（macOS / Linux）：可写作 `Function`，可与普通键组合，但不能与 `F1`–`F24` 组合。Linux 上 `Fn` 通过 evdev 读取；配置了 `Fn` 的热键在未指定后端时自动选择 Wayland/evdev 后端。
- 功能键 `F1`–`F24`，如 `F9`、`Alt+F8`。
- 非文本控制键和导航键：`Tab`、`Enter`、`Escape`、`Backspace`、`CapsLock`、方向键、`Home`、`End`、`PageUp`、`PageDown`、`Insert`、`Delete`。
- 不支持：`Fn+F1`–`Fn+F24`，以及不带 `Fn` 的字母、数字、标点、`Space`、小键盘键等会输入文本的按键（如 `Alt+G`、`G`、`Alt+1`、`Alt+Space`）。

Windows 上低级键盘钩子只观察按键边沿，不消费、不回放修饰键，因此单独的 `Alt`、`Super` 和 `Alt+Tab` 等系统快捷键保持原有行为。

### TUI 快捷键

| 按键 | 行为 |
| --- | --- |
| `j` / `k` / 方向键 | 移动选中项 |
| `e` / `i` / `Enter` | 编辑选中项（字符串字段进入输入；选项字段用 `j`/`k` 切换；布尔字段用 `Space` 切换）；“引擎”/“引擎配置”行回车打开弹层 |
| `Enter` | 退出编辑并保存；`Esc` 仅退出编辑 |
| `s` | 保存全部配置（即时生效，无需重启） |
| `l` | 展开/收起日志区域 |
| `h` | 帮助开关 |
| `q` / `Ctrl+C` | 退出 |

在 TUI 中修改引擎或引擎凭证后按 `s` 保存，下一次录音即使用新配置；正在进行的识别仍使用建立连接时的配置。

## 配置

<details>
<summary>展开查看配置文件路径与推荐配置</summary>

默认配置路径：

```text
# Linux / macOS
~/.config/just-talk/config.toml

# Windows
%APPDATA%\just-talk\config.toml
```

同时会优先读取当前目录下的 `./config.toml` 和 `$XDG_CONFIG_HOME/just-talk/config.toml`。

推荐配置：

```toml
[voice]
enabled = true
mode = "toggle"                    # toggle | hold
push_to_talk = "Alt+Super"
language = "zh-CN"
auto_submit = true                 # 识别完成后自动上屏
stop_delay_ms = 0                  # hold 模式松开后的停止延迟
device = ""                        # 录音设备（留空使用默认）
gain = 0                           # 麦克风增益
app_key = "your-app-key"
access_key = "your-access-key"
resource_id = "volc.bigasr.sauc.duration"
hotwords = ["Wayland", "Sway", "wl-copy", "wtype", "just-talk-go"]

# 引擎：doubao-stream（豆包流式，默认）/ dashscope（千问 Fun-ASR 流式）
#       openai / auralwise / mimo-asr（整段转写，无实时字幕）
engine = "doubao-stream"
# 各引擎的凭据与参数独立保存，互不影响：
# [voice.engine_configs.dashscope]
# api_key = "sk-..."
# model = "fun-asr-realtime"

[overlay]
enabled = true
# macOS 默认 "notch"，其他平台默认 "bottom-center"
# 可选：notch、top-left、top-center、top-right、bottom-left、bottom-center、bottom-right
position = "notch"
show_text = true                   # 刘海中是否显示实时/最终识别文字
idle_visible = false               # 空闲时是否显示提示
scale = 1.0

[debug]
enabled = false
hotkeys = []                       # 调试插件额外注册的热键
```

`position = "notch"`（macOS 默认）让提示从刘海展开，`show_text` 控制刘海内是否显示实时/最终文字；浮层固定在鼠标所在屏幕，屏幕无刘海时回退为顶部居中胶囊（不显示文字），Linux / Windows 上 `"notch"` 按 `top-center` 处理。预览与效果说明见上文 [macOS 刘海浮层](#macos-刘海浮层)。在 TUI 中选中“提示位置”按 `e` 进入编辑、`j`/`k` 切换、`Enter` 保存即可即时修改；已有配置中显式写了 `position` 的不会被迁移。

</details>

## 命令行

<details>
<summary>展开查看 CLI 参数</summary>

```text
just-talk [flags]

-tui                 启用 TUI（默认）
-no-tui              后台模式，日志输出到 stderr
-backend <name>      强制后端：x11 | wayland | darwin（也可设 JUST_TALK_BACKEND）
-config <path>       指定配置文件路径
-doctor              运行启动环境检查后退出
-install             为当前用户安装可执行文件
-debug               启用调试插件（配合配置 [debug]）
-verbose             详细日志
```

</details>

## 开发

<details>
<summary>展开查看构建、测试与调试命令</summary>

```bash
make build          # 构建当前平台
make run            # 直接运行
go test ./...       # 运行全部测试
go test ./... -tags no_x11    # 无 X11 环境下测试
goreleaser check    # 校验 .goreleaser.yaml
```

可选的 Windows 集成测试（需要真实设备/钩子环境）：

```bash
JUST_TALK_TEST_WINDOWS_AUDIO=1 go test ./plugins/voice -run TestWindowsRecorderIntegration -v
JUST_TALK_TEST_WINDOWS_HOTKEY=1 go test ./hotkey -run TestWindowsHookIntegration -v
```

TUI 模式下普通日志只写入日志文件与界面内的日志区域，避免破坏 Bubble Tea 布局；调试详情仅在 `--debug` 时可见。

</details>

## 更新日志

见 [CHANGELOG.md](CHANGELOG.md)。

## 维护与贡献

本仓库是 [whoamihappyhacking/just-talk-go](https://github.com/whoamihappyhacking/just-talk-go) 的独立 fork。上游项目仍由原作者维护；本仓库独立演进，目标是社区驱动的开发，欢迎所有贡献者参与。

欢迎通过 [Issue](https://github.com/wakaka6/just-talk-go/issues) 反馈 bug、使用体验和功能建议；也欢迎 Pull Request。贡献流程与要求见 [CONTRIBUTING.md](CONTRIBUTING.md)：建议先开 Issue 讨论改动方向，提交前运行 `gofmt`、`go vet` 和 `go test ./...`。

## 许可证

Just Talk 使用 [GNU General Public License v3.0](LICENSE)（GPL-3.0-only）开源。
