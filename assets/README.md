# Assets Directory

This directory already contains the icon assets and bundled WAV sounds used by the Go application.

## Required Assets

- `icon.png` - Application icon (copy from Python version and convert SVG to PNG)
- `icon.svg` - SVG version of icon
- `start.wav` - Sound played when recording starts
- `stop.wav` - Sound played when recording stops  
- `done.wav` - Sound played when transcription completes

## If you need to refresh assets from the older Python version:

```bash
# From the voice-to-clipboard-go directory:
cp ../voicetoclipboard/assets/*.wav ./assets/
cp ../voicetoclipboard/assets/icon.svg ./assets/

# Convert SVG to PNG (requires imagemagick or similar):
convert ../voicetoclipboard/assets/icon.svg -resize 256x256 ./assets/icon.png
# Or with inkscape:
inkscape -w 256 -h 256 ../voicetoclipboard/assets/icon.svg -o ./assets/icon.png
```
