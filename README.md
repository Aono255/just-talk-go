# Just Talk

[English](README.en.md) · [下载与版本](https://github.com/Aono255/just-talk-go/releases) · [问题反馈](https://github.com/Aono255/just-talk-go/issues)

减少用键盘的次数，改用口喷吧。

Just Talk 是通过全局快捷键使用的桌面语音输入工具：录音后将识别文字复制到剪贴板，或直接粘贴到当前输入框。macOS 上还可以接入 DeepSeek 等第三方模型，结合当前 Codex 聊天自动纠错、整理后上屏。

本仓库由 [Aono255](https://github.com/Aono255/just-talk-go) 独立维护，基于 [whoamihappyhacking/just-talk-go](https://github.com/whoamihappyhacking/just-talk-go/tree/master)，维护分支为 `master`，提供自己的 Release 和 Homebrew 更新。

[快速开始](#快速开始) · [Codex 纠错](#codex-上下文纠错macos) · [三种词表](#三种词表怎么选) · [浮窗设置](#macos-浮窗) · [配置与命令](#配置文件与命令) · [常见问题](#常见问题)

## 现在能做什么

| 功能 | 使用方式与范围 |
| --- | --- |
| 全局语音输入 | `toggle` 按一次开始、再按一次停止；`hold` 按住录音、松开停止 |
| 流式识别与上屏 | 使用火山豆包语音识别；支持自动粘贴或仅复制到剪贴板，自动上屏不会按回车发送消息 |
| Codex 上下文纠错 | **macOS 专属，默认关闭**。识别后读取当前聊天，调用 DeepSeek 或 OpenAI 兼容服务，校对后上屏 |
| 独立 AI 术语表 | 较完整的项目名、技术词、英文缩写供纠错模型参考，与识别热词分开维护 |
| 本地与云端识别热词 | 支持直传热词，以及火山自学习平台的热词表 ID；云端词表需要在客户端绑定 |
| 中文录音浮窗 | macOS 显示音量波形、识别文字、AI 整理动效和完成结果，并突出可见的改动区域 |
| TUI 配置与统计 | 可滚动的配置列表、帮助与日志、配置保存和重载，以及次数、总字数、平均/最近速度统计 |
| 取消与重新录音 | `Esc` 取消当前流程；错误提示期间按 `R` 发起一次新录音 |
| 安装与诊断 | macOS Homebrew、三个平台的发布包、环境检查和 Codex 上下文检查 |

开启“Codex 纠错”后，**微信等其他应用仍使用普通语音输入**，不读取 Codex 聊天、不调用纠错模型；无需反复关闭开关。Linux 和 Windows 支持基础语音输入，当前不支持 Codex 上下文纠错。

macOS 原生浮窗示例（使用演示文案）：

| AI 整理中 | 已完成，突出可见改动 |
| --- | --- |
| ![AI 整理浮窗](docs/overlay-ai-correction.png) | ![纠错完成浮窗](docs/overlay-completed.png) |

## 快速开始

### macOS：安装与更新

Homebrew 预编译版本需要 **macOS 15 或更高版本**，支持 Apple Silicon 和 Intel。

```bash
brew tap Aono255/just-talk https://github.com/Aono255/just-talk-go
brew install Aono255/just-talk/just-talk
just-talk --version
just-talk
```

首次必须按上面的命令显式添加 tap：配方放在本仓库，tap 名称是 `Aono255/just-talk`。

之后更新：

```bash
brew update
brew upgrade Aono255/just-talk/just-talk
just-talk --version
```

更新后退出旧 Just Talk 并重新启动，运行中的进程不会自动切换到新版本。同一时间只运行一个实例。

如果已经安装 `wakaka6/tap/just-talk`，先在原终端按 `Ctrl+C` 退出，再运行 `brew uninstall wakaka6/tap/just-talk`，随后安装本 fork。常规卸载保留用户配置，切换后检查原有配置即可。

### 首次录音

1. 在终端启动 `just-talk`。macOS 的辅助功能和麦克风权限应授予**实际启动它的终端应用**，例如 Terminal 或 iTerm2。辅助功能入口为“系统设置 → 隐私与安全性 → 辅助功能”。
2. 在火山引擎的豆包语音应用中开通相应识别服务，取得 App ID 和 Access Token。在 TUI 的 **App Key** 填 App ID，在 **Access Key** 填 Access Token。这两项用于语音识别，与纠错模型的 API Key 分开配置。
3. 使用 `j/k` 选择配置项、`e` 编辑；按 `Enter` 结束编辑并保存，或按 `Esc` 结束编辑后再按 `s` 保存。
4. 回到目标应用，把光标放在输入框。默认热键为 `Alt+Super`：macOS 对应 `⌥ Option + ⌘ Command`，Windows 的 `Super` 对应 Win 键。
5. 默认 `toggle` 模式下按一次开始、再按一次停止。开启“自动上屏”时自动粘贴；关闭时手动粘贴剪贴板内容。

App Key、Access Key、识别热词和纠错设置保存后，下一次录音使用新值；已开始的录音继续使用其凭证和词表快照。安装了新二进制或修改浮窗参数后需要重启。

### Linux 与 Windows

从 [Release](https://github.com/Aono255/just-talk-go/releases) 下载对应系统、架构的归档，解压后运行 `just-talk` 或 `just-talk.exe`。提供 Linux/macOS 的 amd64、arm64，以及 Windows 的 amd64、arm64；校验文件为 `SHA256SUMS.txt`。

| 平台 | 基础语音输入的依赖与权限 |
| --- | --- |
| Linux Wayland | `alsa-utils` 提供录音；`wl-clipboard` 提供剪贴板；Sway/wlroots 通常使用 `wtype` 上屏。热键需要读取 `/dev/input/event*`；KDE Plasma 上屏需注意 `/dev/uinput` 写权限 |
| Linux X11 | `alsa-utils`、`xclip`；使用原生 X11 热键与 XTest 上屏 |
| macOS | 原生热键、AVFoundation 录音、剪贴板和 AppKit 浮窗；为启动终端授予辅助功能与麦克风权限 |
| Windows 10/11 | 原生热键、WinMM 录音、Unicode 剪贴板、SendInput 上屏与状态浮窗；在麦克风隐私设置中允许桌面应用访问 |

Linux 的依赖和权限可先用 `just-talk --doctor` 检查。Windows 不需要额外的 ffmpeg、SoX 或 C 编译器。

## 日常操作

### TUI 按键

| 按键 | 操作 |
| --- | --- |
| `j/k` 或方向键 | 选择配置项；列表会随选中项滚动 |
| `e`、`i` 或 `Enter` | 进入选中项的编辑模式 |
| 编辑开关时按空格 | 切换开/关 |
| 编辑选项时按 `j/k` | 切换 `toggle/hold`、模型服务类型等选项 |
| 编辑时按 `Enter` | 结束编辑并保存配置 |
| 编辑时按 `Esc`，再按 `s` | 结束编辑后保存；`Esc` 本身不会保存或撤销已编辑的字段 |
| `h` | 显示/隐藏配置帮助 |
| `l` | 展开/收起日志 |
| `q` 或 `Ctrl+C` | 在导航模式退出程序 |

录音、等待识别或纠错期间，全局 `Esc` 可取消当前流程；麦克风正在启动时会标记取消，等待系统调用返回后释放设备。错误提示约保留 10 秒，期间全局 `R` 用于**重新录音**，不会重放上一次声音或重跑上一次纠错。

### 热键、模式与停止延迟

默认配置为 `Alt+Super` + `toggle`。`hold` 适合按住说话；停止延迟表示停止操作后继续补录的毫秒数，默认 `0`。

支持纯修饰键、`F1`–`F24`，以及 `Tab`、`Enter`、`Escape`、`Backspace`、`CapsLock`、方向键和导航键。示例：`Alt+Super`、`F9`、`Alt+F8`、`Ctrl+Alt+Tab`。不接受字母、数字、标点、空格或数字小键盘字符，避免将正常文本输入注册为录音热键。

`Option` 等价于 `Alt`，`Command/Cmd/Win` 等价于 `Super`。Windows 的修饰键组合要求精确匹配；低级键盘钩子只观察按键，不消费或回放修饰键，普通 `Alt`、Win 和 `Alt+Tab` 仍按系统规则工作。

## Codex 上下文纠错（macOS）

### 接入 DeepSeek 或其他模型

纠错默认关闭。启用后沿用录音热键，流程为：

```text
Codex：录音 → 语音识别 → 读取当前聊天 → 模型纠错整理 → 校验 → 自动粘贴或仅复制
其他应用：录音 → 语音识别 → 自动粘贴或仅复制
```

在 TUI 中填写下列项目，再开启“Codex 纠错”并保存：

| TUI 项目 | DeepSeek 默认值或用途 |
| --- | --- |
| 模型服务类型 | `deepseek`；其他兼容服务选择 `openai-compatible` |
| 模型服务地址 | `https://api.deepseek.com`；其他服务填写自己的 Base URL，可能含 `/v1`，不要填写完整 `/chat/completions` 地址 |
| 纠错模型 | `deepseek-flash`；按服务商提供的模型名填写 |
| 模型 API Key | 在本地填写该服务的 Key，查看与编辑时均隐藏 |
| 纠错超时(ms) | 默认 `8000`，允许 `500`–`30000` |
| AI 纠错术语 | 中英文逗号分隔，例如 `Codex, DeepSeek, SOCKS5, WebShell, ClickHouse` |

DeepSeek 模式显式关闭思考并启用 JSON 输出；默认模型和参数见 [DeepSeek 官方接口说明](https://api-docs.deepseek.com/api/create-chat-completion/)。其他服务需兼容 `POST /chat/completions`、Bearer 认证和 `choices[].message.content`，模型结果须为约定的 JSON 文本。程序直接发出 HTTP 请求并复用连接，不启动 Codex CLI，不自动切换模型或重试到其他服务。

默认 8 秒是模型请求的超时上限，**不代表每次处理都要等 8 秒**。上下文读取、识别收尾和粘贴另有耗时；实际速度和效果取决于服务、网络、识别原文与上下文。TUI 显示发送的上下文数量和字符数，以及成功纠错的耗时。

### 读取多少上下文

只读取**当前 Codex 草稿框所属聊天主区域内已载入的消息**，按可访问的角色标题选取：

| 内容 | 上限 |
| --- | --- |
| 用户消息 | 最近 6 条，每条最多 1200 字符 |
| 助手回复 | 最近 3 条，每条最多 2800 字符 |
| 总上下文 | 最多 15600 字符，保持原对话顺序 |

长消息保留首尾并标注中间省略。程序不汇总其他聊天，也不读取仓库文件；虚拟列表尚未载入的历史不可用。TUI 展示实际发送量，日志记录已载入数量和发送量，不保存上下文正文。

使用时先点进当前聊天的草稿框。普通助手流式回复可以继续增长；切换应用、切换聊天、用户消息锚点或草稿发生变化会阻止整理结果粘贴。无法确认上下文时明确失败，不借用其他聊天或执行无上下文纠错。

检查读取是否正常：

```bash
just-talk --check-codex-context
```

运行后五秒内回到 Codex 当前聊天并点击草稿框。检查只报告已载入/实际选用的消息数量、字符数、输入框绑定和耗时，不录音、不访问模型服务、不保存消息正文。

### 纠错范围与失败处理

模型校对同音错字、技术词、明显语病、标点和分段，并删除不影响意思的停顿与重复，保持原意和语气。热词或 AI 术语中已配置的含数字名称可恢复，例如 `SOCKS` → `SOCKS5`。

原有数字、编号、代码、URL 和附件标记仍会校验。`G01` 改为 `M01`、改动端口、删除或重复原有编号会被拒绝。模型给可确认词汇新增的单反引号排版会被去掉，原文已有代码格式保留。

服务错误、超时、校验失败或输入目标变化时，保留原始识别文字到剪贴板，显示具体错误并停止自动上屏；可手动粘贴原文。按 `Esc` 或开始新录音可取消进行中的模型请求。误词和上下文不足仍可能影响纠错质量，术语表不能保证模型每次都恢复正确。

## 三种词表怎么选

| 配置入口 | 作用阶段 | 如何填写 |
| --- | --- | --- |
| “热词” / `voice.hotwords` | 语音识别，也作为 AI 纠错的补充词汇 | TUI 用中英文逗号；配置文件用字符串数组 |
| “火山热词表 ID” / `voice.boosting_table_id` | 火山云端语音识别 | 先在自学习平台创建词表，再填写对应 ID |
| “AI 纠错术语” / `correction.terms` | 仅供纠错模型参考，不发给语音识别 | 适合完整项目名、缩写和含数字标准名称；TUI 用中英文逗号 |

在 TUI 填写词表时会去除空项并去重；本地热词和 AI 术语是两个独立配置，需要分别维护。AI 术语不受识别词表预算约束，但仍占用模型输入，可能影响耗时与费用。DeepSeek 模式会在内存中的本地热词列表补入 `DeepSeek`，不改写已保存的词表；未配置云端 ID 时才随直传热词发送给识别服务。

### 使用火山自学习平台

1. 进入火山控制台“豆包语音 → 自学习平台 → 热词管理”，选择与 Just Talk App Key 对应的应用。
2. 添加热词文件，上传 UTF-8 TXT 或填写热词内容，创建后复制热词 ID。
3. 在 TUI 的“火山热词表 ID”填入 ID 并保存，下一次录音生效。只在云端创建不会自动绑定 Just Talk。

上传文件使用**每行一个词**，不要直接上传逗号分隔的 AI 术语表。例如：

```text
Codex
DeepSeek
WebShell
```

官方词表支持每表最多 5000 词、每词少于 10 个字；含阿拉伯数字或特殊符号的词需按规则改成对应汉字。可用 `词|权重` 指定 1–10 的权重，默认 4；标准名称如 `SOCKS5` 可以另外保留在 AI 术语中。具体限制见 [官方热词管理说明](https://www.volcengine.com/docs/6561/155739?lang=zh)。

配置了云端 ID 后，识别请求只发送该 ID，不同时发送优先级更高的本地直传热词；清空 ID 后恢复本地热词。此时本地热词仍供 AI 纠错参考，云端词表内容不会自动下载到 AI 术语表。

## macOS 浮窗

浮窗依次显示“准备中、录音中、收尾中、识别中、AI 整理中、已完成”，错误显示“处理失败”及原因。

- 录音波形使用实际录音流的音量，不额外打开麦克风或采集系统声音。
- AI 阶段显示蓝紫动效和“上下文纠错”标签；完成后突出可见的修改区域，并保留结果约 3 秒。
- 录音、识别、AI 整理和完成使用相同的 448×104 点展开尺寸，随 `scale` 缩放；AI 阶段不再额外放大。
- 长句在固定两行区域显示能容纳的最近文字，完整结果仍会复制或粘贴。
- 浮窗不抢焦点、不接收鼠标，隐藏后停止动画，并遵循系统“减少动态效果”。

浮窗设置目前通过配置文件修改，**重启 Just Talk 后生效**：

```toml
[overlay]
enabled = true
position = "notch"
idle_visible = false
scale = 1.0
```

默认位置为 `bottom-center`。通用位置包括 `top-left`、`top-center`、`top-right`、`bottom-left`、`bottom-center`、`bottom-right`；macOS 的 `notch` 位于屏幕顶部中央、菜单栏下方，也可用于无刘海外接显示器。`idle_visible = true` 让待机提示保留，`scale` 调整整体大小。

## 配置文件与命令

默认保存位置为 Linux/macOS 的 `~/.config/just-talk/config.toml`、Windows 的 `%APPDATA%\just-talk\config.toml`。

未指定 `--config` 时，读取顺序是当前目录的 `config.toml`、平台默认路径、`$XDG_CONFIG_HOME/just-talk/config.toml`；找到第一个现有文件后使用它。使用 `--config` 时，TUI 保存回指定文件。macOS/Linux 保存权限为 `0600`。

下面是包含当前配置项的示例；凭证留空，词表仅为演示，启用语音识别前填写火山凭证，启用纠错前填写模型 Key：

```toml
[voice]
enabled = true
mode = "toggle"
push_to_talk = "Alt+Super"
app_key = ""                           # 火山 App ID
access_key = ""                        # 火山 Access Token
resource_id = "volc.bigasr.sauc.duration"
auto_submit = true
stop_delay_ms = 0
hotwords = ["Codex", "DeepSeek", "WebShell"]
boosting_table_id = ""                 # 留空使用本地热词
device = ""                            # macOS 仅支持空值/default
gain = 1                               # 整数 PCM 增益，低于 1 按 1 处理
language = "zh-CN"                     # 保留字段，见下方说明

[correction]
enabled = false
provider = "deepseek"                  # 或 openai-compatible
base_url = "https://api.deepseek.com"
model = "deepseek-flash"
api_key = ""
timeout_ms = 8000
terms = ["Codex", "DeepSeek", "SOCKS5", "WebShell", "ClickHouse"]

[overlay]
enabled = true
position = "bottom-center"
idle_visible = false
scale = 1.0

[debug]
enabled = false
hotkeys = []
```

`voice.device` 在 Linux 可填 ALSA 设备名，在 Windows 可填默认值、设备编号或完整设备名。macOS 使用系统当前默认输入，不修改输入/输出设备或硬件采样率。`language` 当前保留在配置中，但未发送到识别请求，不能据此强制识别语言。

| 命令 | 用途 |
| --- | --- |
| `just-talk` | 默认 TUI 模式 |
| `just-talk --no-tui` | 无 TUI 运行，仍使用原热键；`Ctrl+C` 退出 |
| `just-talk --version` | 查看二进制版本与提交 |
| `just-talk --config ./trial.toml` | 使用指定配置，并将 TUI 修改保存回该文件 |
| `just-talk --doctor` | 检查平台依赖、权限等；不实际录音，也不验证服务端 Key 或识别效果 |
| `just-talk --check-codex-context` | macOS 当前 Codex 上下文与输入框绑定检查 |
| `just-talk --verbose` | 在日志中记录详细的热键、麦克风启动和收尾信息 |
| `just-talk --debug` | TUI 中显示热键接收、排队与处理诊断 |
| `just-talk --backend wayland` / `--backend x11` | 在 Linux 指定后端；也支持 `JUST_TALK_BACKEND` |
| `just-talk --install` | 安装当前二进制到 Linux/macOS 的 `~/.local/bin` 或 Windows 的 `%LOCALAPPDATA%\Programs\Just Talk` |

无 TUI 模式下，额外的调试热键日志需要同时指定 `--debug` 并设置 `[debug].enabled = true`，通过 `debug.hotkeys` 选择要观察的组合。

Linux/macOS 日志为 `/tmp/just-talk.log`，Windows 为 `%LOCALAPPDATA%\just-talk\just-talk.log`。次数、字数和速度是统计信息，不是可回放的录音历史。TUI 会显示识别文字；无 TUI 模式会输出识别文字，分享诊断内容时请自行检查需要隐藏的文字。

## 常见问题

### 开启 DeepSeek 后，微信也提示要聚焦 Codex？

先确认 `just-talk --version` 至少为 `v0.1.9`，并退出旧进程后重新启动。新版通过单次助手进程查询当前前台应用，微信等应用直接使用普通语音输入。如果提示发生在 Codex 内，需要点击当前聊天草稿框后再录音。

`v0.1.8` 使用的系统级 AX 焦点查询曾在已授权的实机上立即返回 `-25204`，延长等待仍失败，提示“无法确认当前输入应用”。`v0.1.9` 改用独立进程中的 AppKit 应用身份查询；助手不读取配置、聊天正文或麦克风，失败或超过 2 秒仍明确停止上屏。Codex 的输入框与聊天绑定继续由主进程检查，上屏前再次查询应用身份。失败原因写入诊断日志，便于区分应用检测与聊天读取问题。

### 一直停在“准备中”或旧版的 CON？

检查当前默认麦克风和蓝牙耳机是否可用，以及其他应用是否在采集系统声音。此前蓝牙耳机排查中，关闭 Codex 的 Audio visualizer 后连续录音恢复正常；可在提供该选项的版本中检查“设置 → 通用 → Toys”。这是已验证的排查线索，不保证覆盖所有设备。

macOS 在收到首个音频缓冲后才进入录音状态。启动调用返回后等待首个缓冲的上限为 3 秒，**这个限制不能终止阻塞中的系统启动调用**。在启动中按 `Esc` 会标记取消，系统返回后释放资源；不要连续打开多个实例。

### 热键不响应？

确认“语音输入”已开启并保存；macOS 检查启动终端的辅助功能权限，Linux Wayland 检查 input 权限。使用 `--debug` 查看热键是否已接收，结合 `--verbose` 区分“没有收到热键”和“热键收到后卡在麦克风启动”。本 fork 会恢复被系统因超时禁用的 macOS 热键监听；用户主动禁用监听时仍保持禁用。

### 提示模型改动了数字、代码、URL 或附件？

这是结果保护校验。原始识别已保留在剪贴板，可先粘贴原文。把需要恢复的完整名称放入 AI 术语，例如 `SOCKS5`；`v0.1.7` 起支持按已配置词表恢复含数字名称，并处理可确认词汇的额外反引号。普通数字、原有编号和代码仍受保护，错误类别会显示在提示中。

### 浮窗文字显示不全，或者 AI 阶段没有出现？

浮窗只有两行预览区域，完整文字仍会正常上屏或复制。AI 整理只在 macOS、开关开启且当前输入应用为 Codex 时出现；读取上下文失败时直接显示错误。修改浮窗位置、缩放或开关后需要重启。

### 内容会发送到哪里？

录音音频发送到火山识别服务。只有在 Codex 纠错流程中，识别文字、选中的聊天上下文、本地热词和 AI 术语才会发送到配置的模型服务。模型 API Key 保存在本地配置并用于请求认证，不写入日志；语音服务和模型服务分别使用自己的凭证。

## 从源码构建与维护

需要 Go 1.25 或更高版本。Linux 和 macOS 使用原生 cgo，Windows 使用直接 Win32 调用；项目不提供非 cgo macOS 版本。

```bash
git clone https://github.com/Aono255/just-talk-go.git
cd just-talk-go
```

Linux 编译库（录音/剪贴板运行依赖另外见前面的平台表）：

```bash
# Arch Linux
sudo pacman -S --needed gcc libx11 libxtst libxext libxinerama libxrender wayland

# Debian / Ubuntu
sudo apt install build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libxrender-dev libwayland-dev
```

macOS 安装 Apple Command Line Tools，不需要完整 Xcode：

```bash
xcode-select --install
```

Linux/macOS 构建和安装：

```bash
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
build/just-talk --install
# 确保 ~/.local/bin 已加入 PATH
```

Windows PowerShell（Go 1.25+，不需要 C 编译器）：

```powershell
go build -o build\just-talk.exe .\cmd\just-talk
.\build\just-talk.exe --install
# 按命令提示将安装目录加入 PATH
```

可将 `go build -o` 的输出和 `GOCACHE`、`GOMODCACHE` 指向自选构建盘；macOS 必须在本机 macOS 上构建。测试命令为 `go test ./...`、`go vet ./...`，打包配置检查为 `goreleaser check`。

维护者推送稳定的 `v主版本.次版本.修订版本` 标签后，工作流在六个系统/架构组合原生测试和构建，发布归档与校验文件，自动更新 `Formula/just-talk.rb`，再在 Intel 和 Apple Silicon 上验证实际 Homebrew 安装。Formula 由 `scripts/update_homebrew.py` 生成，详细维护规则见 [AGENTS.md](AGENTS.md)。

跟进原项目时合并 `upstream/master`，保留本 fork 的修复，不强制同步覆盖。原项目不接受 PR；本 fork 的问题请提交到 [Issues](https://github.com/Aono255/just-talk-go/issues)。版本变更见 [CHANGELOG.md](CHANGELOG.md)。

## 原项目网页与许可证

[原项目介绍页](https://whoamihappyhacking.github.io/just-talk-go/) 展示上游功能；本 fork 保留 `website/` 源码，但未启用自身 Pages 自动部署，当前功能以本 README 为准。可用 `python3 -m http.server 7788 --bind 127.0.0.1 --directory website` 在本地查看介绍页和不调用麦克风的演示。

Just Talk 使用 [GNU General Public License v3.0](LICENSE)。
