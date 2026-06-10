# Assets Directory

Icon assets, UI sounds, and the VAD model embedded into the application
binary via `go:embed` (see `main.go`).

- `icon.svg` / `icon.png` - Application icon (PNG used by the system tray)
- `start.wav` - Sound played when recording starts
- `stop.wav` - Sound played when recording stops
- `done.wav` - Sound played when transcription completes
- `silero_vad_v6.onnx` - Silero VAD model (MIT), extracted to the cache dir
  on first run

To regenerate the PNG from the SVG:

```bash
magick assets/icon.svg -resize 256x256 assets/icon.png
# or: inkscape -w 256 -h 256 assets/icon.svg -o assets/icon.png
```
