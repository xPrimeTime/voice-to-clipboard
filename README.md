# Voice to Clipboard - Go Edition

Local voice-to-text transcription with automatic clipboard integration. Go desktop app with local Whisper/CTranslate2 transcription.

## Quick Start

### Download Binary (Easiest)

1. Go to [Releases](https://github.com/yourusername/voice-to-clipboard/releases)
2. Download for your platform:
   - **Linux**: `voice-to-clipboard-linux-amd64`
   - **Windows**: `voice-to-clipboard-windows-amd64.exe`

3. **Linux**:
   ```bash
   chmod +x voice-to-clipboard-linux-amd64
   ./voice-to-clipboard-linux-amd64
   ```

4. **Windows**: Double-click `voice-to-clipboard-windows-amd64.exe`

5. On first run, if no model is present, the app downloads `base` automatically and shows progress in the UI.
6. You can switch models later from the tray menu:
   - **Tiny** (~75MB) - Fastest, multilingual
   - **Base** (~145MB) - Fast, multilingual
   - **Small** (~486MB) - Better accuracy, multilingual
   - **Medium** (~1.5GB) - Great accuracy, multilingual
   - **Large-v3** (~3GB) - Best accuracy, multilingual

### Build from Source

#### Prerequisites
- Go 1.23 or later
- CTranslate2 library (for transcription)
- libwhisper_ct2.so (from go-whisper-ct2 v1.2.0)
- libonnxruntime.so (for the bundled Silero v6 VAD) — resolved via the
  go-whisper-ct2 install/rpath; bundled by the distribution scripts

The Silero VAD model (`assets/silero_vad_v6.onnx`) is embedded in the binary and
extracted to the cache dir on first run, so no separate VAD download is needed.

**Linux**:
```bash
# Install CTranslate2
pip install ctranslate2  # or from source

# Build and install libwhisper_ct2.so
# (See go-whisper-ct2 repository for details)
```

#### Build
```bash
git clone https://github.com/yourusername/voice-to-clipboard-go
cd voice-to-clipboard-go
bash scripts/build.sh
```

Binary will be in `build/bin/voice-to-clipboard`

## Features

- 🎤 **Local transcription** - No internet required, 100% private
- ⚡ **Fast startup** - Under 100ms from launch to UI
- 💾 **Lightweight UI** - Small desktop app with modest idle footprint
- 🖥️ **Minimal UI** - Tiny floating window (120x28px)
- 📋 **Auto-clipboard** - Transcribed text automatically copied
- 🔔 **Notifications** - Desktop notifications on completion
- 🎯 **Cross-platform** - Linux (Wayland/X11) and Windows
- 🔊 **Audio feedback** - Sounds for start/stop/done
- 🚀 **Optimized performance** - Auto-tunes CPU threads to your physical cores

## Usage

### Global Hotkey

**Ctrl+Shift+R** - Toggle recording from anywhere (Windows/X11)

- Works system-wide, even when window is hidden
- **Windows**: Native global hotkey support
- **Linux X11**: Global hotkey via robotn/gohook
- **Linux Wayland**: Use compositor keybinds (see below)

### Controlling the app (works on any compositor)

The most portable way to drive the app is the **IPC command layer**. A second
launch with a control flag talks to the running instance over its IPC socket
instead of opening a new window — these work identically on X11, Wayland, any
compositor, and regardless of whether the system tray works:

| Command                        | Action                          |
|--------------------------------|---------------------------------|
| `voice-to-clipboard --toggle`  | Start/stop recording            |
| `voice-to-clipboard --quit`    | Quit the running instance       |
| `voice-to-clipboard --show`    | Show the window                 |
| `voice-to-clipboard --hide`    | Hide the window                 |

Bind them to compositor keys. **Hyprland** (`~/.config/hypr/hyprland.conf`):
```bash
bind = SUPER, R,        exec, /path/to/voice-to-clipboard --toggle
bind = SUPER SHIFT, Q,  exec, /path/to/voice-to-clipboard --quit
bind = SUPER SHIFT, S,  exec, /path/to/voice-to-clipboard --show
```
**Sway** (`~/.config/sway/config`) / **i3** use `bindsym $mod+r exec …` the same way.

### In-App Shortcuts

- **Space / Enter**: Toggle recording (when window focused)
- **Escape**: Close window (or hide if auto-hide enabled)
- **Ctrl+Q**: Quit application

### Workflow

1. Launch the app (or use system tray)
2. Click mic button or press **Space** to start recording
3. Speak your text
4. Press **Space** again to stop
5. Wait for transcription (notification when done)
6. Text is automatically in your clipboard - just paste!

### System Tray

The tray icon is a **status indicator** plus a best-effort menu:
- **Left-click** the icon: start/stop recording.
- **Right-click**: menu with Toggle Recording, Auto-hide, Keep hidden, Model
  selection, Quit.

> **Tray portability:** tray hosts vary a lot. Some Wayland panels (e.g. AGS)
> show the icon but **don't deliver menu clicks**, others (waybar) need a `tray`
> module, and GNOME needs an extension. So don't rely on the tray menu — use the
> IPC commands above (bound to keys) as the primary control. The icon's
> left-click (toggle recording) and the in-app shortcuts always work.

## Configuration

Settings are stored in:
- **Linux**: `~/.config/voice-to-clipboard/config.json`
- **Windows**: `%APPDATA%\VoiceToClipboard\config.json`

### Change Model

Edit `config.json`:
```json
{
  "model": "small",
  "auto_hide": true,
  "keep_hidden": false
}
```

Or change via system tray menu.

### Model Cache

Models are cached at:
- **Linux**: `~/.cache/voice-to-clipboard/models/`
- **Windows**: `%LOCALAPPDATA%\VoiceToClipboard\cache\models\`

## Performance Optimization

### CPU threads (automatic)

On startup the app detects your **physical core count** and sets
`OMP_NUM_THREADS` to it (unless you already set the variable). This keeps
CTranslate2's CPU kernels on physical cores instead of oversubscribing SMT/
hyperthread siblings, which measured **~35% faster** on the Whisper encoder
(e.g. ~1.75s → ~1.17s encode on an 8-core/16-thread Ryzen).

To override the automatic value, set `OMP_NUM_THREADS` yourself:

```bash
# Linux/macOS - add to ~/.bashrc or ~/.zshrc, or run once per session
OMP_NUM_THREADS=8 ./voice-to-clipboard
```

Physical cores is a good default; going above it (into SMT threads) usually
slows transcription rather than speeding it up.

### Compute type

On CPU the app uses CTranslate2's `default` compute type, which runs the
float16 faster-whisper weights as **float32**. `int8` would be faster but
requires a CPU/CTranslate2 build with efficient int8 support (e.g. AVX-512 VNNI);
where it is unavailable, model loading fails, so `default` is used.

### Use a smaller model

`base` is roughly 3× faster than `small` (the default). Switch via the tray or
`config.json` if you want lower latency and can accept slightly lower accuracy.

**Rule of thumb**: Use 75% of your total thread count for best performance.

Without this setting, transcription will be ~2.3x slower than optimal.

## Development

See [ARCHITECTURE.md](ARCHITECTURE.md) for technical documentation.

### Project Structure

```
voice-to-clipboard-go/
├── main.go                  # Entry point
├── internal/
│   ├── ui/                  # Gio UI (minimal floating window)
│   ├── audio/              # Recording and playback
│   ├── transcribe/         # CTranslate2 integration
│   ├── tray/               # System tray
│   ├── system/             # Clipboard, notifications
│   ├── config/             # Configuration management
│   └── logger/             # Logging
├── assets/                 # Icons and sounds
└── build/                  # Build outputs
```

## Troubleshooting

### Linux: No audio input
```bash
# Check microphone
arecord -l

# Ensure PipeWire/PulseAudio running
systemctl --user status pipewire
```

### Linux: Clipboard not working (Wayland)
```bash
# Install wl-clipboard
sudo pacman -S wl-clipboard  # Arch
sudo apt install wl-clipboard  # Ubuntu
```

### Model download fails
Check internet connection and try again. Models download from HuggingFace and may be slow.

### Transcription fails with "Failed to open tokenizer file"
The model directory is missing `tokenizer.json`, which go-whisper-ct2 v1.2.0+
requires. Models downloaded by older versions only have `model.bin`,
`config.json`, and a vocabulary file. Fix by re-downloading the model (delete
its folder under `~/.cache/voice-to-clipboard/models/<model>/` and reselect it),
or fetch the file directly:

```bash
cd ~/.cache/voice-to-clipboard/models/<model>
curl -fLO https://huggingface.co/Systran/faster-whisper-<model>/resolve/main/tokenizer.json
```

New downloads include `tokenizer.json` automatically.

### Tray menu / Quit does nothing (Wayland)
Some Wayland panels don't deliver tray menu clicks to the app. Quit reliably
with `/path/to/voice-to-clipboard --quit` or the IPC keybinds. See
[System Tray](#system-tray).

### Hyprland: window fills the whole screen (fullscreen overlay)
The 120×40 window must **float**; a tiled window fills the workspace on
Hyprland. The app forces floating via `hyprctl dispatch setfloating` when it
shows the window, but for guaranteed behavior from the moment it launches, add
a float rule to `~/.config/hypr/hyprland.conf` (syntax differs by version):

```bash
# Hyprland 0.4x+ (unified windowrule with match:)
windowrule = float on, match:class ^(voice-to-clipboard)$

# Older Hyprland (windowrulev2)
# windowrulev2 = float, class:^(voice-to-clipboard)$
```

Then `hyprctl reload`.

(Optionally `size 120 40` and a `move` rule to pin its position.)

### Build/test in restricted environments
If `go build` or `go test` fails because the default Go build cache is not writable, point `GOCACHE` at a writable directory:

```bash
GOCACHE=/tmp/go-build go build ./...
GOCACHE=/tmp/go-build go test ./...
```

## Credits

- UI built with [Gio](https://gioui.org/)
- Transcription via [CTranslate2](https://github.com/OpenNMT/CTranslate2)
- Go bindings: [go-whisper-ct2](https://github.com/xPrimeTime/go-whisper-ct2)
- Based on the original Python version

## License

MIT License - see LICENSE file

## Contributing

Contributions welcome! Please open an issue first to discuss changes.

1. Fork the repo
2. Create a feature branch
3. Make your changes
4. Submit a pull request

## Roadmap

- [ ] GPU acceleration (CUDA/Metal)
- [ ] Multiple language support
- [ ] Settings UI
- [ ] Hotword detection
- [ ] Custom keybinds
- [ ] Plugin system

---

**Note**: This is a Go rewrite of the original Python version focused on native UI, local transcription, and simpler packaging.
