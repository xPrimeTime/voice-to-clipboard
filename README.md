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
- libwhisper_ct2.so (from go-whisper-ct2)

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
- 🚀 **Optimized performance** - Set `OMP_NUM_THREADS` for optimal speed

## Usage

### Global Hotkey

**Ctrl+Shift+R** - Toggle recording from anywhere (Windows/X11)

- Works system-wide, even when window is hidden
- **Windows**: Native global hotkey support
- **Linux X11**: Global hotkey via robotn/gohook
- **Linux Wayland**: Use compositor keybinds (see below)

### Compositor Keybinds (Wayland/Any)

For Wayland or if you prefer compositor-managed hotkeys:

**Hyprland** (`~/.config/hypr/hyprland.conf`):
```bash
bind = SUPER, R, exec, /path/to/voice-to-clipboard --toggle
```

**Sway** (`~/.config/sway/config`):
```bash
bindsym $mod+r exec /path/to/voice-to-clipboard --toggle
```

**i3/i3wm** (`~/.config/i3/config`):
```bash
bindsym $mod+r exec --no-startup-id /path/to/voice-to-clipboard --toggle
```

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

Right-click the tray icon for:
- Toggle Recording
- Auto-hide window on close
- Keep window hidden (tray-only mode)
- Model selection
- Quit

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

For **optimal transcription speed** (~1.8x faster), set the `OMP_NUM_THREADS` environment variable:

```bash
# Linux/macOS - Add to ~/.bashrc or ~/.zshrc
export OMP_NUM_THREADS=12  # Use ~75% of your CPU threads

# Or run once per session
OMP_NUM_THREADS=12 ./voice-to-clipboard
```

**Recommended values by CPU:**
- 16-thread CPU (8 cores): `OMP_NUM_THREADS=12`
- 8-thread CPU (4 cores): `OMP_NUM_THREADS=6`
- 4-thread CPU (2 cores): `OMP_NUM_THREADS=3`

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
