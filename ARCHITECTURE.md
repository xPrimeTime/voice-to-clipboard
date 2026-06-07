# Voice to Clipboard - Gio Architecture Documentation

Complete technical documentation for the Gio-based Voice to Clipboard application.

## Table of Contents
- [Overview](#overview)
- [Technology Stack](#technology-stack)
- [Architecture Design](#architecture-design)
- [Module Details](#module-details)
- [Concurrency Model](#concurrency-model)
- [Cross-Platform Considerations](#cross-platform-considerations)
- [Performance Characteristics](#performance-characteristics)

---

## Overview

Voice to Clipboard is a local speech-to-text application built with Go and Gio that provides:

- **Local transcription** using CTranslate2 via `go-whisper-ct2` (no cloud API required)
- **Native UI** using Gio (OpenGL-based immediate mode GUI)
- **Minimal floating window** - 120×40dp with mic button and audio visualizer
- **System tray integration** for background operation
- **Cross-platform** support (Linux primary, Windows secondary)
- **Portable bundle distribution** with runtime libraries and model download on first run

### Key Goals

1. **Simple Distribution**: App bundle with runtime libraries, minimal setup
2. **Fast Startup**: <100ms from launch to UI
3. **Low Memory**: <50MB idle, <700MB during transcription
4. **Native Performance**: No webview overhead
5. **Open Source**: Clean, maintainable codebase

---

## Technology Stack

### Core Technologies

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.23+ | Static compilation, fast, simple concurrency |
| UI Framework | Gio | Native OpenGL rendering, truly native, no webview |
| Transcription | CTranslate2 + faster-whisper models (`go-whisper-ct2`) | Fast CPU inference with production-ready model runtime |
| Audio Capture | malgo | Cross-platform, low-level audio access |
| Audio Playback | oto/v3 | Simple, reliable, cross-platform |
| Clipboard | atotto/clipboard | Cross-platform clipboard access |
| Notifications | gen2brain/beeep | Native notifications on Linux/Windows |
| System Tray | fyne.io/systray | Cross-platform system tray |
| Hotkey | robotn/gohook | Global hotkey listener (Windows/X11) |

### Why Gio?

**Gio over other UI frameworks**:
- ✅ Truly native (OpenGL rendering, no webview)
- ✅ Portable bundle distribution
- ✅ Immediate mode GUI (simple mental model)
- ✅ Excellent performance for small UIs
- ✅ Minimal runtime requirements when bundled
- ✅ Active development

---

## Architecture Design

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Main Application (main.go)                │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Gio UI Event Loop                         │ │
│  │  - Window Management                                   │ │
│  │  - UI Rendering (immediate mode)                       │ │
│  │  - User Input Handling                                 │ │
│  └────────────────────────────────────────────────────────┘ │
│                           │                                  │
│  ┌────────────────────────┼────────────────────────────┐    │
│  │    Application State   │                            │    │
│  │  - Recording status    │                            │    │
│  │  - Audio levels        │                            │    │
│  │  - Configuration       │                            │    │
│  └────────────────────────┼────────────────────────────┘    │
└────────────────────────────┼─────────────────────────────────┘
                             │
            ┌────────────────┼─────────────────┐
            │                │                 │
   ┌────────▼──────┐  ┌──────▼──────┐  ┌──────▼───────┐
   │ Audio Module  │  │  Transcribe  │  │System Module │
   │               │  │    Module    │  │              │
   │ - Recording   │  │              │  │ - Clipboard  │
   │ - Playback    │  │ - CTranslate2│  │ - Notify     │
   │ - VAD         │  │ - Models     │  │ - Window Mgmt│
   │ - Levels      │  │              │  │              │
   └───────────────┘  └──────────────┘  └──────────────┘
            │                 │                 │
   ┌────────▼──────┐  ┌───────▼───────┐ ┌──────▼───────┐
   │ Config Module │  │ Logger Module │ │ Hotkey Module│
   │               │  │               │ │              │
   │ - Settings    │  │ - slog-based  │ │ - Global     │
   │ - Models list │  │ - File output │ │   hotkeys    │
   └───────────────┘  └───────────────┘ └──────────────┘
            │
   ┌────────▼──────┐
   │  Tray Module  │
   │               │
   │ - Status      │
   │ - Menu        │
   └───────────────┘
```

### Data Flow

#### Recording Flow
```
User clicks mic button
    ↓
main.go: startRecording()
    ↓
audio.Recorder.Start()
    ↓
Audio captured in background goroutine
    ↓
Audio levels calculated for visualizer
    ↓
Audio data accumulated in buffer
    ↓
User clicks stop
    ↓
main.go: stopRecording()
    ↓
audio.Recorder.Stop() returns PCM float32 buffer
    ↓
worker.Transcribe(audioData)
    ↓
Text result returned
    ↓
clipboard.WriteAll(text)
    ↓
UI shows success animation
```

#### UI Update Flow (Gio Immediate Mode)
```
Event occurs (recording start/stop, audio level update)
    ↓
State updated in main.go
    ↓
w.Invalidate() called to request redraw
    ↓
Gio calls layout function
    ↓
UI rebuilt from current state
    ↓
Rendered to screen
```

---

## Module Details

### main.go - Application Entry Point

**Responsibilities:**
- Gio UI window creation and management
- Application state management
- Coordinating between modules
- UI rendering (immediate mode)
- Event handling

**Key Functions:**
- `main()`: Entry point, initializes all modules, starts UI
- `ui.Run()`: Gio event loop
- `startRecording()`: Initiates recording
- `stopRecording()`: Stops recording and triggers transcription

**State:**
```go
type AppState struct {
    recording    bool
    audioLevels  [4]int
    processing   bool
    config       *config.Config
    // ... other state
}
```

### internal/ui/ - UI Components

**Responsibilities:**
- Gio widget implementations
- Visual elements (buttons, visualizer, etc.)
- Layout logic

**Key Components:**
- `MicButton`: Circular microphone button with state
- `AudioVisualizer`: 4-bar audio level display
- `ThemedButton`: Reusable styled button
- Layout helpers

### internal/audio/ - Audio Capture & Processing

**Responsibilities:**
- Audio recording via malgo
- Audio playback (success/error sounds)
- Real-time audio level calculation (RMS, for the visualizer)

**Key Functions:**
- `NewRecorder()`: Initialize audio recorder
- `Recorder.Start()`: Begin capture
- `Recorder.Stop()`: Stop and return audio buffer
- `CalculateBarHeights()`: Compute visualizer levels
- `CalculateRMS()`: Per-frame RMS, used for the level meter

Note: Voice Activity Detection is no longer performed in this package. It is
delegated to the transcription library's Silero VAD (see below).

### internal/transcribe/ - Speech Recognition

**Responsibilities:**
- CTranslate2 model management (via `go-whisper-ct2` v1.2.0)
- Model downloading from Hugging Face
- Audio transcription
- Voice Activity Detection (Silero v6 ONNX VAD)

**Key Functions:**
- `NewWorker()`: Initialize transcription worker
- `Worker.Transcribe()`: Convert audio buffer to text
- `ModelManager.DownloadModel()`: Fetch model from Hugging Face
- `Worker.EnsureModelLoaded()`: Load/reload active model after config changes
- `ExtractVADModel()`: Materialize the embedded Silero VAD model to the cache dir

**Voice Activity Detection:**
When `VADEnabled` is set, transcription passes `WithVADFilter(true)` plus
`WithVADModel(<silero_vad_v6.onnx>)` to the library, which runs faster-whisper's
Silero v6 VAD to trim non-speech. The Silero model is bundled (`assets/`,
embedded via `//go:embed`) and extracted to the cache dir at startup because
onnxruntime requires a real file path. If the model can't be materialized, the
library falls back to its built-in energy VAD. Long dictations (>30s) benefit
automatically from the v1.2.0 long-audio seek logic.

**Supported Models:**
- tiny (~75MB, fast, less accurate)
- base (~150MB, balanced) - downloaded on first run
- small (~500MB, good quality) - config default
- medium (~1.5GB, high quality)
- large-v3 (~3GB, best quality)

### internal/config/ - Configuration Management

**Responsibilities:**
- Load/save user configuration
- Default configuration
- Model information database

**Config Structure:**
```go
type Config struct {
    Model          string
    ModelCachePath string
    SampleRate     int
    Channels       int
    AutoHide       bool
    KeepHidden     bool
    Language       string
    VADEnabled     bool
}
```

### internal/system/ - System Integration

**Responsibilities:**
- Clipboard operations
- Desktop notifications
- Window management (hide/show)
- Hyprland-specific features

**Key Functions:**
- `CopyToClipboard()`: Write text to clipboard
- `ShowNotification()`: Display desktop notification
- `HideWindow()`: Move window to special workspace (Hyprland)
- `ShowWindow()`: Restore window visibility

### internal/hotkey/ - Global Hotkey

**Responsibilities:**
- Register global keyboard shortcuts
- Platform-specific implementations

**Platform Support:**
- `hotkey_unix.go`: Linux/macOS implementation using gohook (X11 support on Linux)
- `hotkey_windows.go`: Windows implementation using gohook

### internal/tray/ - System Tray

**Responsibilities:**
- System tray icon and menu
- Status updates
- Quick actions (show/hide, exit)
- Model selection submenu

**Menu Structure:**
```
Status: Ready
---
Start Recording
---
Auto-hide window
Keep window hidden
---
Models
    tiny (~75MB)
    base (~145MB)
    small (~486MB)
    medium (~1.5GB)
    large-v3 (~3GB)
---
Quit
```

### internal/logger/ - Logging

**Responsibilities:**
- Structured logging using Go's slog
- File output to ~/.local/share/voice-to-clipboard/app.log (XDG_DATA_HOME)

---

## Concurrency Model

### Goroutines Used

1. **Gio UI Thread** (main goroutine)
   - Handles all UI events and rendering
   - Must not block

2. **Audio Recording** (background)
   - Continuously captures audio data
   - Writes to circular buffer
   - Calculates audio levels

3. **Transcription** (spawned per request)
   - Runs CTranslate2 inference
   - Can take 1-10 seconds depending on model
   - Returns result via channel

4. **Hotkey Listener** (background)
   - Monitors for global hotkey events
   - Signals main thread via channel

5. **System Tray** (background)
   - Handles tray menu events
   - Uses callback functions

### Communication Patterns

**Channels and synchronization:**
- Recording/transcribing state: `atomic.Bool`
- Audio buffer: `sync.Mutex`
- UI state and bar heights: `sync.RWMutex`
- Audio levels: `chan float32`
- Stop/cancel signals: `chan struct{}`
- UI redraws: Gio `Invalidate()`

---

## Cross-Platform Considerations

### Linux (Primary Target)

**Window Management:**
- Basic X11/Wayland support via Gio
- Special Hyprland integration for hide/show
- Detects Hyprland compositor and uses special workspace

**Audio:**
- PulseAudio/PipeWire via malgo
- Auto-selects default input device

### Windows (Secondary Target)

**Window Management:**
- Native Win32 window via Gio
- No special compositor features

**Audio:**
- WASAPI via malgo
- Device selection via default input

### macOS

There is path handling and a `darwin` build tag in the hotkey package, but this repo is not documented or tested as a supported macOS target.

---

## Performance Characteristics

### Memory Usage

| State | Memory | Notes |
|-------|--------|-------|
| Idle | 30-50MB | Model not loaded |
| Recording | 50-80MB | Audio buffer + UI |
| Transcribing (tiny) | 200-300MB | Model loaded |
| Transcribing (base) | 300-500MB | Model loaded |
| Transcribing (small) | 700-900MB | Model loaded |
| Transcribing (medium) | 1.5-2GB | Model loaded |
| Transcribing (large) | 3-4GB | Model loaded |

### Startup Time

- Cold start: 50-100ms (to UI visible)
- Model load: 100-500ms (first transcription)
- Hot transcription: 1-10s depending on model and audio length

### CPU Usage

- Idle: 0%
- Recording: 1-2% (one core)
- Transcribing: 80-100% (CPU-bound, uses multiple cores)

---

## Build & Distribution

### Build Process

```bash
# Linux
go build -o voice-to-clipboard .

# Windows bundle (from a Windows build host)
powershell -ExecutionPolicy Bypass -File scripts/build-windows-bundle.ps1 -Version dev

# Smaller binary
go build -ldflags="-s -w" -o voice-to-clipboard .
```

### Binary Size

- Linux: ~15MB (with debug symbols)
- Linux: ~10MB (stripped with -ldflags="-s -w")
- Windows: ~15MB

### Dependencies

**Runtime:** Go binary plus CTranslate2 runtime libraries (`libwhisper_ct2`, the
CTranslate2 libs, and `libonnxruntime` — required by the Silero v6 VAD). The
bundle scripts allowlist `libonnxruntime` so it ships alongside the CT2 libs.

**Build-time:**
- Go 1.23+
- C compiler (gcc/clang)
- Gio dependencies (OS-specific):
  - Linux: X11/Wayland dev libraries
  - Windows: MinGW-w64

### First Run Behavior

1. Creates config directory: `~/.config/voice-to-clipboard/` (Linux)
2. Initializes default config (`small` is the config default)
3. If no model is present on disk, auto-downloads `base` and shows progress in the UI
4. Starts tray and global hotkey handlers when supported

---

## Design Decisions

### Why Immediate Mode UI (Gio)?

**Advantages:**
- Simple mental model: UI is a pure function of state
- No state synchronization between UI and app logic
- Easy to reason about: redraw = rebuild from state
- Perfect for small, dynamic UIs

**Trade-offs:**
- Every frame redraws everything (fine for small UI)
- Less efficient than retained mode for large UIs
- Different paradigm than web/native GUI

### Why No Window Position Saving?

- 120x28px window is meant to be repositioned frequently
- Users typically place it near their work area
- Saving position adds complexity for minimal benefit

### Why VAD is Optional?

- Some users want manual control (long recordings)
- VAD can be too aggressive in noisy environments
- Toggle allows flexibility
- When enabled, VAD is the library's Silero v6 ONNX model (faster-whisper
  parity), not a Go-side energy heuristic

### Why Full-Buffer Transcription?

- The current pipeline records locally, then transcribes after stop
- Streaming adds more UI and inference complexity
- Current code is optimized for short command-style recordings

---

## Future Enhancements

Potential improvements:

1. **GPU Acceleration**: Expanded CUDA/Metal support through CTranslate2 runtime options
2. **Streaming**: Real-time transcription as you speak
3. **Multiple Languages**: Auto-detect or manual selection
4. **Custom Hotkeys**: User-defined keyboard shortcuts
5. **Themes**: Dark/light mode, custom colors
6. **Plugins**: Extensibility via Go plugins

---

## Summary

Voice to Clipboard uses Gio for a native UI experience and CTranslate2 for local speech recognition. The architecture is centered around a single main event loop that coordinates audio recording, transcription, and UI updates. All modules are cleanly separated with clear responsibilities, making the codebase maintainable and testable.

Key architectural strengths:
- **Simple delivery**: App binary plus bundled runtime libraries
- **Clean separation**: Modules communicate via Go channels and interfaces
- **Native performance**: Gio's OpenGL rendering is fast and efficient
- **Simple concurrency**: Clear goroutine ownership and communication

The design prioritizes simplicity and maintainability over feature richness, resulting in a focused tool that does one thing well: local voice-to-text with minimal friction.
