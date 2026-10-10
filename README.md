# Just Talk

[English](README.en.md) · [项目主页](https://whoamihappyhacking.github.io/just-talk-go/)

减少用键盘的次数，改用口喷吧。

Just Talk 是一个面向桌面环境的语音输入工具。它通过全局快捷键录音，把语音识别结果复制到剪贴板，或直接上屏到当前输入框，适合写代码、聊天、记笔记和处理长文本输入。

本仓库是 [Aono255 维护的 fork](https://github.com/Aono255/just-talk-go)，代码基于 [原项目的 master](https://github.com/whoamihappyhacking/just-talk-go/tree/master)。我们使用 `master` 作为维护分支，独立发布版本和 Homebrew 更新。

在 TUI 中修改 App Key 或 Access Key 后，退出编辑模式并按 `s` 保存；下一次录音会使用新凭证，无需重启。正在识别的录音继续使用建立连接时的凭证。

## 截图

![Just Talk TUI](docs/screenshot-tui.png)

## 功能

- 全局快捷键录音，支持 `toggle` 和 `hold` 两种模式。
- 语音热键限定为适合作为全局快捷键的按键：支持纯修饰键、功能键、Tab、CapsLock、方向键和导航键等；不支持字母、数字、标点、空格等普通字符键。
- 豆包大模型流式 ASR，支持双向流优化版和二遍识别。
- 自动复制到剪贴板，支持自动上屏。
- Wayland / X11 / macOS / Windows 顶层录音状态胶囊提示。
- macOS 浮层使用中文状态、音量波形和 AI 整理动效，识别结束后保留结果 3 秒。展开后录音、识别、AI 整理和完成均为 448×104 点，随配置缩放；AI 阶段不额外放大。长句在固定的两行预览区域保留能容纳的最近文字，完整结果正常粘贴。`notch` 位置显示在屏幕顶部中央、菜单栏下方，也支持无刘海外接显示器。
- TUI 配置界面，支持热键、模式、自动上屏、停止延迟、热词等配置。
- 热词增强识别，适合项目名、人名、英文术语和专有名词。
- 录音历史统计，包括历史次数、总字数、平均速度和最近速度。

## 平台状态

当前支持 Linux、macOS 和 Windows 桌面：

| 平台 | 状态 | 说明 |
| --- | --- | --- |
| Linux Wayland | 已支持 | 已支持 Sway / wlroots 场景；快捷键基于 evdev，需要 input 权限 |
| Linux X11 | 已支持 | 使用 X11 原生全局热键 |
| macOS | 已支持 | 全局快捷键基于 CGEventTap，录音使用 AVFoundation，剪贴板使用 NSPasteboard，胶囊显示使用 AppKit NSPanel |
| Windows 10/11 | 已支持 | 全局按键轮询及低级键盘钩子边沿回退、WinMM 录音、Unicode 剪贴板、SendInput 自动上屏和 Win32 状态胶囊 |

## macOS：Homebrew 安装与更新

首次安装（macOS 15 或更高版本，Apple Silicon 和 Intel 均提供预编译版本）：

```bash
brew tap Aono255/just-talk https://github.com/Aono255/just-talk-go
brew install Aono255/just-talk/just-talk
just-talk --version
```

如果已经安装 `wakaka6/tap/just-talk`，先在运行它的终端按 `Ctrl+C`，再执行 `brew uninstall wakaka6/tap/just-talk`，然后安装上面的版本。卸载命令不带 `--zap`，用户配置 `~/.config/just-talk/config.toml` 保留；同一时间只运行一个 Just Talk。

之后更新：

```bash
brew update
brew upgrade Aono255/just-talk/just-talk
```

更新完成后重启正在运行的 Just Talk 进程。`brew update` 更新 Formula，`brew upgrade` 安装新版本；正在运行的旧进程不会自动切换。

macOS 热键监听被系统因超时禁用时，本 fork 会自动重新启用监听。用户主动禁用监听时保持禁用。此路径有原生回调回归测试；后台长时间使用后的实际热键表现仍需试用确认。

v0.1.3 在后台打开麦克风，立即显示“正在打开麦克风”，打开期间仍处理松开和取消；已松开或取消的启动不会在返回后开始识别，也不会重复创建采集会话。macOS 从打开到释放麦克风期间使用系统活动声明，标识用户发起的录音及高优先级 I/O，结束后立即释放。

macOS 使用 AVFoundation 采集用户当前选择的默认麦克风，输出 16 kHz、16 位单声道 PCM。它在收到首个音频缓冲后才进入录音状态；会话启动后 3 秒仍未收到音频会报告错误，这个等待不限制系统启动调用本身的耗时。系统默认输入、输出和麦克风硬件采样率不做修改；设备不可用时直接报告错误，不选择其他麦克风。`voice.device` 在 macOS 只支持空值或 `default`。

蓝牙麦克风如果长期停在 `CON`，请检查其他应用的系统声音采集功能。在 macOS 26.5.1 的实机排查中，AudioQueue 和 AVFoundation 都曾卡在 Core Audio 等待耳机设备启动的位置，麦克风授权仍然有效。关闭 Codex「设置 → 通用 → Toys → Audio visualizer」并重新打开 JustTalk 后，连续录音、识别和上屏成功，启动至收到音频约 162–453 毫秒。这支持排查系统声音采集与蓝牙设备重配置的相互影响；更换录音 API 本身不能保证消除此类系统等待。无需修改麦克风权限，也不要让 JustTalk 自动关闭其他应用的采集。

采集回调把音频暂存于内存，等待识别连接时不会因管道写满而阻塞；超过 1 MiB 未读数据（约 32 秒）时明确报告缓冲错误。停止时保留已采集的尾部数据，唤醒读取线程并在其退出后释放原生资源。`--verbose` 记录设备查找、配置、启动和首个缓冲的分段耗时。原生回归测试覆盖格式、缓冲和停止生命周期；其他耳机与系统组合仍需各自验证。

## 构建

Just Talk 依赖平台原生能力。Linux 和 macOS 构建需要启用 cgo；Windows 使用纯 Go 的 Win32 调用，不需要 cgo。

Linux 构建依赖：

```bash
# Arch Linux
sudo pacman -S --needed go gcc libx11 libxtst libxext wayland

# Debian / Ubuntu
sudo apt install golang-go build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libwayland-dev
```

macOS 构建依赖：

```bash
# 需要 Apple Command Line Tools 提供 clang 和 macOS SDK；不需要安装完整 Xcode。
xcode-select --install
```

Windows 构建依赖：

```powershell
# 安装 Go 1.25 或更高版本；不需要额外安装 ffmpeg、SoX 或 C 编译器。
winget install --id GoLang.Go --exact
```

构建当前平台二进制：

```bash
cd just-talk-go
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
```

Windows PowerShell：

```powershell
cd just-talk-go
go build -o build\just-talk.exe .\cmd\just-talk
```

安装到 `~/.local/bin/just-talk`：

```bash
# 确保 ~/.local/bin 在 PATH 中（如未配置，将下面这行加入 ~/.bashrc 或 ~/.zshrc）
# export PATH="$HOME/.local/bin:$PATH"
build/just-talk --install
# 或
make install
```

macOS 需要在本机 macOS 上构建；项目不提供非 cgo 版本。

Windows 安装到 `%LOCALAPPDATA%\Programs\Just Talk\just-talk.exe`：

```powershell
.\build\just-talk.exe --install
# 如果安装目录尚未在 PATH 中，按命令输出提示添加即可。
```

## Release 下载

[本 fork 的 GitHub Release](https://github.com/Aono255/just-talk-go/releases) 提供以下预编译归档：

- Linux amd64 / arm64
- macOS Intel / Apple Silicon
- Windows amd64 / arm64
- `SHA256SUMS.txt` 文件校验

发布流程使用 GoReleaser v2 和官方 `goreleaser/goreleaser-action`。Linux、macOS 和 Windows 二进制分别在对应的 GitHub 托管 runner 上原生构建；维护者推送 `v*` 标签时会自动构建并发布，例如：

```bash
git tag v0.1.3
git push origin master v0.1.3
```

使用稳定版本标签 `v主版本.次版本.修订版本`。所有平台构建和测试成功后，工作流发布归档与校验值，再自动提交本仓库的 `Formula/just-talk.rb`。它只使用 GitHub 内置令牌，不需要额外 PAT。Formula 当前提供 macOS 安装；Linux 和 Windows 使用 Release 归档或自行构建。

发布后会在干净的 Apple Silicon 和 Intel macOS runner 上执行实际的 `brew tap`、`brew install` 和 `brew test`。也可以手动运行“Homebrew 安装验证”工作流检查当前配方。

跟进原项目时，先将 `upstream` 指向 `https://github.com/whoamihappyhacking/just-talk-go.git`，再执行 `git fetch upstream master` 和 `git merge upstream/master`；验证后提交到本仓库并发布新标签。不要使用强制同步覆盖本 fork 的修复。

## 使用

默认启动 TUI：

```bash
just-talk
```

后台模式：

```bash
just-talk --no-tui
```

指定后端：

```bash
just-talk --backend wayland
just-talk --backend x11
```

Windows 不需要指定后端。首次使用前可检查麦克风和配置：

```powershell
.\build\just-talk.exe --doctor
```

## 配置

默认配置路径：

```text
# Linux / macOS
~/.config/just-talk/config.toml

# Windows
%APPDATA%\just-talk\config.toml
```

推荐热键配置：

```toml
[voice]
mode = "toggle"
push_to_talk = "Alt+Super"
```

`Alt+Super` 配合 `toggle` 模式是推荐用法。按一次开始录音，再按一次停止录音，避免按住模式下和桌面环境或输入框发生按键冲突。在 Windows 上，低级键盘钩子只观察按键边沿，不会消费或回放修饰键，因此单独使用 `Alt`、`Super` 或 `Alt+Tab` 时会保持系统原有行为。组合键必须精确匹配配置的修饰键集合；钩子回退状态会通过未拦截的物理按键状态校验，避免把两次独立的单键按下拼成组合键。

语音热键只支持适合作为全局快捷键的按键：

- 支持：纯修饰键组合，如 `Alt+Super`、`Ctrl+Alt+Shift`。
- 支持：功能键 `F1` 到 `F24`，如 `F9`、`Alt+F8`。
- 支持：非文本控制键和导航键，如 `Tab`、`Enter`、`Escape`、`Backspace`、`CapsLock`、`Up`、`Down`、`Left`、`Right`、`Home`、`End`、`PageUp`、`PageDown`、`Insert`、`Delete`。
- 不支持：字母、数字、标点、空格、数字小键盘数字和符号等会输入文本的按键，如 `Alt+G`、`G`、`Alt+1`、`Alt+Space`。

热词示例：

```toml
[voice]
hotwords = ["Wayland", "Sway", "wl-copy", "wtype", "just-talk-go"]
```

macOS 热键写法：

```toml
[voice]
# Option 等价于 Alt，Command/Cmd 等价于 Super
push_to_talk = "Option+Command"
```

Windows 使用 `Win` 或 `Super` 表示 Windows 徽标键。如果麦克风不可用，请在“Windows 设置 → 隐私和安全性 → 麦克风”中允许桌面应用访问麦克风。


## Codex 上下文纠错（macOS）

启用后沿用原来的录音热键：识别完成 → 读取当前 Codex 聊天 → 调用配置的小模型纠错和整理 → 自动上屏。macOS 浮层显示“AI 整理中”及“上下文纠错”标签，蓝紫流光在容器内部播放，完成后显示整理结果并突出可见的改动区域。默认关闭；其他应用继续使用普通语音输入。

macOS 浮层的波形音量来自原有录音流，不额外采集麦克风或系统声音。动画在独立 AppKit 辅助进程中播放，不占用热键回调；浮层不抢焦点、不接收鼠标，隐藏后停止动画计时器，并遵循系统“减少动态效果”设置。准备和待机使用小胶囊，进入录音后展开；后续识别、纠错与完成保持相同尺寸和文字位置。

在 TUI 中用 `j/k` 导航至“Codex 纠错”，填写服务地址、模型名和 API Key，再启用并按 `s` 保存。配置列表随光标滚动；API Key 在查看和编辑时均隐藏。服务必须兼容 OpenAI `POST /chat/completions`，以 Bearer Key 认证，并返回 `choices[].message.content`。默认预填 DeepSeek 的地址与 `deepseek-flash`，显式关闭思考并启用 JSON 输出；API Key 留空。选择 `openai-compatible` 可使用其他兼容服务，填写对应地址和模型，不自动切换模型。DeepSeek 当前参数见[官方文档](https://api-docs.deepseek.com/api/create-chat-completion/)。

“AI 纠错术语”是独立词表，可填写较完整的项目名、技术词和缩写，在 TUI 中以中英文逗号分隔；它只发送给纠错模型，不传给语音识别。“热词”仍用于识别，并作为补充词汇发送给纠错模型。AI 术语默认留空，不限制为语音识别的热词预算，但会占用模型输入长度，可能影响耗时与费用。保存后在下一次录音生效，已开始录音使用当时的词表。

```toml
[correction]
enabled = false
provider = "deepseek"                    # 也支持 openai-compatible
base_url = "https://api.deepseek.com"
model = "deepseek-flash"
api_key = ""                           # 在本地填写
timeout_ms = 8000
terms = ["ClawOps", "ReadModel", "两高一弱"] # 独立的 AI 纠错术语
```

上下文只从当前 Codex 草稿框所属的聊天主区域读取，按消息角色取已载入的最近六条用户消息和三条助手回复。每条用户消息最多 1200 字符，每条助手回复最多 2800 字符，总计最多 15600 字符；过长消息保留首尾并标注省略，按原对话顺序发送。只发送这些上下文、本次识别文字及配置的热词。TUI 显示实际发送的条数与字符数，日志另外记录已载入的消息数量，不保存上下文正文。没有读取到消息角色与正文时明确失败，不使用最近活跃的其他聊天。虚拟列表尚未载入的历史无法通过此方式读取。需给启动 JustTalk 的终端辅助功能权限。

启用 DeepSeek 时，每次识别请求会在已配置热词外补入 `DeepSeek`，不改写用户热词设置。助手流式回复的正常增长不会被视为切换聊天；切换输入框、用户消息锚点变化或草稿变化会阻止上屏。

整理只处理本次识别文字：修正同音错字、技术词、明显语病、标点和分段，删除无意义停顿词与重复。数字、代码、URL、附件标记变化时拒绝模型结果。服务错误、超时或聊天/草稿变化时保留原始识别到剪贴板，显示错误并停止自动上屏；按 `Esc` 或开始新录音可取消正在进行的模型请求。配置更改在下一次录音生效。模型调用期间仍可使用热键，不在录音或热键回调中调用模型服务。

使用 `just-talk --check-codex-context` 后，五秒内回到当前 Codex 聊天，把光标放在草稿框；它只报告消息数量、输入框绑定与耗时，不录音、不访问模型服务、不保存消息正文。当前聊天读取及模型真实速度仍需在对应 Codex 版本、服务商和模型上验证。

配置保存权限为 `0600`（macOS/Linux），API Key 仅写入本地配置，不出现在日志中。使用 `--config /path/trial.toml` 启动时，TUI 保存回该文件。接口格式参考 [OpenAI Chat Completions 文档](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)。

## 更新日志

见 [CHANGELOG.md](CHANGELOG.md)。

## 维护与贡献

原项目 Just Talk 由 `whoamihappyhacking` 创建和维护；本 fork 由 `Aono255` 维护。原项目不接受 Pull Request。本 fork 的问题请反馈到[本仓库的 Issues](https://github.com/Aono255/just-talk-go/issues)。

## 许可证

Just Talk 使用 GNU General Public License v3.0 开源。

## 项目介绍网页

在线访问：[原项目的 Just Talk 介绍页](https://whoamihappyhacking.github.io/just-talk-go/)。本 fork 保留网页源码，Pages 自动部署仅在原项目仓库启用。

静态介绍页位于 `website/`，包含功能、平台支持、快速开始与不调用麦克风的交互演示。启动预览：

```bash
python3 -m http.server 7788 --bind 0.0.0.0 --directory website
```
