# Bundle Distribution Guide

This guide is for shipping Voice to Clipboard as portable release archives for open-source users.

It is aligned with the current app behavior and the scripts in `scripts/`:
- `scripts/build-linux-bundle.sh`
- `scripts/build-windows-bundle.ps1`

## Goals

- One archive per platform.
- No end-user dependency install for CT2 runtime libraries.
- Reproducible, testable release output with checksums.
- Optional portable mode that keeps config/cache next to the app.

## Current Runtime Behavior

- If no local model exists, the app auto-downloads `base` on first run.
- Default config model is `small` after config initialization, but first-run bootstrap sets model to `base` when model files are missing.
- By default, config/cache/data are user-profile paths.
- Portable mode is launcher-driven and sets process environment variables.

## Artifacts

### Linux

Artifact:
- `dist/voice-to-clipboard-<version>-linux-amd64.tar.gz`
- `dist/voice-to-clipboard-<version>-linux-amd64.tar.gz.sha256`

Contents:
- `voice-to-clipboard/voice-to-clipboard`
- `voice-to-clipboard/lib/*` (bundled runtime libs)
- `voice-to-clipboard/run.sh`
- `voice-to-clipboard/run-portable.sh`
- `voice-to-clipboard/README.md`
- `voice-to-clipboard/THIRD_PARTY_NOTICES.txt`
- `voice-to-clipboard/LICENSE` (if present)

### Windows

Artifact:
- `dist/voice-to-clipboard-<version>-windows-amd64.zip`
- `dist/voice-to-clipboard-<version>-windows-amd64.zip.sha256`

Contents:
- `voice-to-clipboard/voice-to-clipboard.exe`
- `voice-to-clipboard/*.dll` (bundled runtime DLLs)
- `voice-to-clipboard/run-portable.bat`
- `voice-to-clipboard/README.txt`
- `voice-to-clipboard/THIRD_PARTY_NOTICES.txt`
- `voice-to-clipboard/LICENSE.txt` (if present)

## Prerequisites

### Linux build host

Required tools:
- `go`
- `patchelf`
- `ldd`
- `tar`
- `sha256sum`

Runtime library discovery:
- Script searches standard library locations.
- Script also probes Python `ctranslate2` install paths if `python3` is available.
- You can force locations with env vars:
  - `WHISPER_CT2_LIB_DIR`
  - `CT2_LIB_DIR`

### Windows build host

Required tools:
- PowerShell 5+
- `go`

Runtime library discovery:
- Script scans `PATH` and optional env vars:
  - `WHISPER_CT2_LIB_DIR`
  - `CT2_LIB_DIR`
- If installed, Python `ctranslate2` path is also probed.

## Build Commands

### Linux

```bash
scripts/build-linux-bundle.sh --version 1.0.0
```

Optional output dir:

```bash
scripts/build-linux-bundle.sh --version 1.0.0 --output-dir dist
```

### Windows

```powershell
./scripts/build-windows-bundle.ps1 -Version "1.0.0"
```

Optional output dir:

```powershell
./scripts/build-windows-bundle.ps1 -Version "1.0.0" -OutputDir "dist"
```

## Portable Mode

Portable mode is explicit.

### Linux

```bash
./run-portable.sh
```

This sets:
- `XDG_CONFIG_HOME=./data/config`
- `XDG_CACHE_HOME=./data/cache`
- `XDG_DATA_HOME=./data/share`

### Windows

```bat
run-portable.bat
```

This sets process-scoped:
- `APPDATA=.\data\appdata`
- `LOCALAPPDATA=.\data\localappdata`

If users run the binary directly (`voice-to-clipboard` / `voice-to-clipboard.exe`), OS-default user directories are used.

## Verification Steps (Required Before Release)

### 1. Archive integrity

- Confirm `.sha256` file exists.
- Verify checksum in CI and before upload.

Linux:

```bash
sha256sum -c dist/voice-to-clipboard-<version>-linux-amd64.tar.gz.sha256
```

Windows:

```powershell
$h = Get-FileHash dist\voice-to-clipboard-<version>-windows-amd64.zip -Algorithm SHA256
$h.Hash
```

Compare the printed hash against `dist\voice-to-clipboard-<version>-windows-amd64.zip.sha256`.

### 2. Runtime-link check

Linux:

```bash
tar -xzf dist/voice-to-clipboard-<version>-linux-amd64.tar.gz -C /tmp
LD_LIBRARY_PATH=/tmp/voice-to-clipboard/lib ldd /tmp/voice-to-clipboard/voice-to-clipboard
```

Expected: no `not found` lines.

### 3. Fresh-machine smoke test

On clean VM (or container/VM snapshot):
- Launch app from bundle.
- Confirm first-run model download behavior.
- Record and transcribe a short clip.
- Confirm clipboard copy and notification.
- Confirm tray model switch works.
- Confirm app relaunches cleanly.

### 4. Portable-mode test

- Run launcher (`run-portable.sh` or `run-portable.bat`).
- Confirm config/cache appear under `./data`.
- Confirm no writes to default user profile paths during that run.

## Open-Source Compliance Checklist

Do this for every release:

1. Confirm redistribution rights for bundled runtime libraries (CT2, go-whisper-ct2 runtime libs, MKL/OpenMP/other backend libs).
2. Include your project `LICENSE` in the bundle.
3. Include `THIRD_PARTY_NOTICES.txt`.
4. Keep dependency origins and versions traceable in release notes.
5. Avoid claiming "static/no dependencies" in public docs when runtime shared libraries are bundled.

## Reproducibility Recommendations

- Build from a tagged commit only.
- Pin toolchain versions in CI (Go, Python package versions if used for library discovery).
- Build in clean CI runners for release artifacts.
- Store checksums and attach them to GitHub Releases.

## Suggested Release Pipeline

1. Tag release: `vX.Y.Z`.
2. Build Linux bundle in Linux CI job.
3. Build Windows bundle in Windows CI job.
4. Run smoke tests (at least one clean environment per OS).
5. Publish archives + `.sha256` files.
6. Publish release notes including:
   - commit/tag
   - bundled runtime components summary
   - known limitations

## Known Limitations

- Linux binary compatibility still depends on baseline host ABI (glibc and driver stack expectations).
- Linux bundle verification checks direct dynamic links, but clean-VM smoke testing is still required for tray, audio, clipboard, and display behavior.
- Windows SmartScreen warning is expected for unsigned binaries.
- Windows DLL discovery is conservative; if a backend runtime is missing, add its directory to `PATH`, `WHISPER_CT2_LIB_DIR`, or `CT2_LIB_DIR` and rebuild.
- Model downloads still require network on first run unless models are pre-seeded.

## Maintainer Notes

- Keep this guide in sync with `scripts/build-linux-bundle.sh` and `scripts/build-windows-bundle.ps1`.
- If runtime library names change, update both scripts and this doc together.
