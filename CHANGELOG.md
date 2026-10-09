# Changelog

All notable project changes are tracked here.

## v0.1.3 - 2026-10-09

- macOS 录音改用 AVFoundation，采集当前默认麦克风并输出 16 kHz、16 位单声道 PCM，不更改系统音频设置。
- 记录蓝牙设备启动与系统声音采集的兼容性排查：新旧录音接口都曾在 Core Audio 的设备启动处等待。关闭 Codex 音频可视化并重新打开 JustTalk 后，连续录音、识别和上屏成功，启动至收到音频约 162–453 毫秒；此条件下完成实机验收，不宣称更换接口即可解决所有系统级等待。
- macOS 浮层显示当前录音收到的识别文字，完成后保留 3 秒；长句显示最近 100 个字，旧录音、已取消录音和收尾后的回调不能覆盖当前预览。
- 实现 macOS `notch` 顶部居中位置，修复该配置落到右上角的问题；浮层使用不激活应用的面板并支持全屏空间，AppKit 辅助进程固定在主线程运行。
- 麦克风打开改为后台执行，立即显示启动状态；启动期间热键松开和取消不会被锁阻塞，已松开或取消的启动不再在几十秒后启动识别或回放积压热键。
- macOS 收到首个音频缓冲后才进入录音状态；已启动会话 3 秒未收到音频时报告错误。该等待不限制系统启动调用本身的耗时。
- 音频先保存在内存，不在采集回调中等待管道读端；超过 1 MiB 未读音频明确报告缓冲错误。停止时保留尾部数据、唤醒读取线程，并在其退出后释放原生资源。
- macOS 仅在打开和录音期间使用系统活动声明，标识用户发起的录音及高优先级 I/O；失败和停止均释放活动。
- 详细日志增加设备查找、配置、会话启动及首个音频缓冲的分段耗时。
- 已取消或属于旧录音的停止计时器，不能停止当前恢复或新建的录音。
- 添加模拟慢驱动的启动取消测试及原生采集缓冲测试，覆盖及时处理热键、不重复启动、取消后释放、PCM 格式、停止后的剩余数据、阻塞读取退出和缓冲错误；用户在上述兼容性条件下的重复录音已通过。

## v0.1.2 - 2026-10-09

- 基于 `whoamihappyhacking/just-talk-go` 的 `master` 建立 Aono255 维护分支，独立发布版本。
- macOS 收到热键监听超时禁用通知后自动恢复监听；禁用通知不再读取键盘事件字段，主动停止监听时不会被重新启用。
- 增加不需要辅助功能权限的原生回调回归测试，覆盖超时恢复、用户主动禁用、停止后通知和正常按下/松开事件。
- 添加 `--version`，发布包包含版本号和提交号，便于确认当前运行版本。
- 添加 macOS Homebrew 配方生成和发布后自动更新，支持本 fork 的 `brew update` / `brew upgrade`。
- macOS 预编译包明确以 macOS 15 为最低版本，Homebrew 配方同步声明系统要求。
- Windows 键盘回调与位图内存使用明确的指针类型，保留原生 ABI，消除 `go vet` 的整数地址转换告警。
- 热键恢复路径已通过回归测试；真实后台长时间使用效果仍需人工试用。

## 上游 master（已包含在 v0.1.2）

- Fix TUI configuration reloads retaining old ASR credentials: after saving with `s`, the next recording uses the updated App Key and Access Key without restarting.
- Add a responsive Chinese project introduction website with a simulated recording and R-triggered retry demo, platform-specific launch commands, static HTTP preview on port 7788, and automated GitHub Pages deployment linked from both READMEs.

## v0.0.3 - 2026-07-29

- Reworked Windows modifier-only hotkeys so the low-level hook only observes key edges and never consumes or replays modifiers. `Alt+Super` works through exact physical-state matching, stale hook state is cleared without combining separate single-key presses, and system shortcuts such as `Alt+Tab` remain untouched.

## v0.0.2 - 2026-07-21

- Fixed intermittent Windows `Alt+Super` false activations where stale low-level hook state could make a later Alt-only press look like the full shortcut; Windows modifier combinations now require an exact modifier set and reset stale suppressed state after physical release.

## v0.0.1 - 2026-07-17

- Added a tag-triggered GoReleaser v2 pipeline using the official GitHub Action to build Linux, macOS, and Windows amd64/arm64 archives on native runners and publish SHA256 checksums.
- Added Windows 10/11 support with global key-state monitoring, native WinMM microphone capture, Unicode clipboard access, SendInput auto-submit, a click-through Win32 status overlay, and Windows environment checks.
- Added Windows-standard config, state, log, and per-user installation paths. Windows builds no longer require external recording or clipboard commands.
- Added an opt-in Windows microphone integration test using `JUST_TALK_TEST_WINDOWS_AUDIO=1`.
- Added a low-level Windows keyboard-hook fallback so modifier-only shortcuts such as `Alt+Super` still work when `GetAsyncKeyState` misses the physical Windows key.
- Consume Windows modifier-only voice shortcuts such as `Alt+Super` so they do not activate menus or toolbars in the focused application, while replaying unrelated modifier shortcuts normally.
- Fixed recording shutdown getting stuck while ASR audio writes were still in flight, and accept empty final ASR responses without waiting for a false timeout.
- Redesigned the Windows status overlay with per-pixel transparency, antialiased animated waveform bars, smoother capsule edges, and a soft shadow.
- Clarified README build and install setup steps for the repository directory and `~/.local/bin` PATH.
- Restricted voice hotkeys to non-text global shortcut keys, rejecting letters, digits, punctuation, Space, and similar text-producing keys.
- Avoid duplicate auto-submit on KDE Plasma by using uinput directly and not writing the Wayland primary selection there.
- TUI is now the default startup mode.
- Added persistent usage statistics for total sessions, recognized characters, average speed, and recent speed.
- Added configurable ASR hotwords.
- Added TUI help toggle with `h`.
- Improved Wayland clipboard and auto-submit behavior with `wl-copy` and `wtype`.
- Added Linux recording status overlay for X11 and Wayland.
- Added macOS support for global hotkeys, native recording, clipboard, auto-submit, recording status overlay, and environment checks.
- Removed non-cgo macOS fallback builds; Just Talk now requires cgo for native platform integration.
- Replaced the old Claude-specific agent guide with `AGENTS.md` and clarified build documentation.
- Improved toggle and hold hotkey behavior for fast repeated key presses.
- Show ASR connection and final-result timeout errors in the status UI/overlay instead of immediately falling back to idle.
- Added transient `Esc` cancel and `R` retry hotkeys while recording or showing retryable errors.
- Improved X11 overlay placement on multi-monitor setups and switched X11 rendering to an ARGB window for smoother rounded corners.
- Fixed a Wayland overlay shutdown race that could crash while closing the app, and surfaced Linux `arecord` microphone/device failures in the UI.
- Made `Esc` cancel active overlay states, including the final ASR wait state, and suppress output from canceled pending sessions.
- Improved Wayland overlay rounded-corner antialiasing, especially on KDE Plasma.
- Added `just-talk --install` and `make install` to install the binary into `~/.local/bin`.

## 2026-05-30

- Initial Linux-focused development snapshot.
- Supported Linux Wayland hotkeys via evdev.
- Supported Linux X11 hotkeys via native X11 grabs.
- Added Doubao streaming ASR integration.
- Added TUI configuration interface.
- Added automatic clipboard copy and auto-submit.
