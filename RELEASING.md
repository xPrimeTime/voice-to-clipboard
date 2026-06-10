# Releasing

How a release of Voice to Clipboard is built and what must be checked before
tagging. Releases are built **locally** with the bundle scripts — there is no
release CI (the old `.github/workflows/release.yml` was a leftover from the
pre-Gio Wails prototype and was removed; don't resurrect it without rewriting
it for the CTranslate2 native stack).

## Build the artifacts

```bash
# Linux (run on Linux; needs patchelf + the go-whisper-ct2/CTranslate2 dev libs)
bash scripts/build-linux-bundle.sh --version 0.1.0
# -> dist/voice-to-clipboard-0.1.0-linux-amd64.tar.gz (+ .sha256)

# Windows (run on a Windows machine; needs MinGW-w64 and the CT2 DLLs —
# set WHISPER_CT2_LIB_DIR / CT2_LIB_DIR if they aren't on PATH)
powershell scripts/build-windows-bundle.ps1 -Version 0.1.0
# -> dist/voice-to-clipboard-0.1.0-windows-amd64.zip (+ .sha256)
```

Both scripts stamp the version into the binary (`--version` prints it), strip
symbols, and on Windows build with `-H=windowsgui` so no console window opens.

## Pre-tag checklist (Linux v0.1)

1. **Rebuild the bundle from the release commit.** A `dist/` tarball lying
   around may predate recent commits — check `tar -xzf`'d binary with
   `--version` and rebuild if in doubt.

2. **Clean-machine library check** (needs sudo for docker). The bundle
   deliberately does NOT ship desktop platform libs (ALSA, X11, Wayland, EGL,
   zlib — every desktop distro has them), so install those in the container
   first; everything else must resolve from the bundle's `lib/`:

   ```bash
   tar -xzf dist/voice-to-clipboard-0.1.0-linux-amd64.tar.gz -C /tmp
   sudo docker run --rm -v /tmp/voice-to-clipboard:/app:ro archlinux:latest \
       bash -c "pacman -Sy --noconfirm alsa-lib libx11 libxcb \
                libxkbcommon libxkbcommon-x11 libxcursor libxfixes libglvnd \
                wayland zlib >/dev/null && \
                ldd /app/voice-to-clipboard | grep 'not found' && echo FAIL \
                || echo PASS"
   ```

   `PASS` = every library resolved (the binary's rpath points at the
   bundle's `lib/`). Don't *execute* the app in the container: gohook's C
   constructor segfaults without an X display before `main()` runs (see
   Known gaps), which looks like a failure but says nothing about the
   libraries. GUI/audio/tray can only be fully tested on a real desktop.

   The container must satisfy the bundle's **glibc floor**, which the build
   script measures and prints (and writes into the bundle's README). Built on
   this Arch host the floor is glibc 2.43, hence the `archlinux` image —
   `ubuntu:22.04` (glibc 2.35) fails with symbol-version errors. See the
   glibc note under Known gaps.

3. **Licensing sign-off** — done for v0.1 (2026-06-10): every redistributed
   component verified as MIT, BSD, Apache-2.0, Unlicense, LGPL (dynamically
   linked, replaceable .so files), or GPL with the GCC Runtime Library
   Exception; Silero VAD v6 is MIT. `THIRD_PARTY_NOTICES.txt` (written by
   the bundle scripts) lists each component with license and URL. Re-check
   only if the dependency set changes. v0.2 nicety: ship full license texts
   instead of URLs.

4. **README pass**: point the "Download Binary" quick-start at the actual
   GitHub release URL, and check feature docs are current (drag-to-move,
   `--version`, IPC flags).

5. **Tag and publish**:

   ```bash
   git tag v0.1.0 && git push origin master v0.1.0
   # create the GitHub release manually, upload dist/*.tar.gz + .sha256
   ```

## Known gaps (accepted for v0.1, fix later)

- **glibc floor is build-host dependent — currently 2.43 (Arch). Accepted
  for v0.1** (2026-06-10): first release ships with the documented floor
  (stated in the bundle README and the project README); lowering it is a
  v0.2 goal. The
  native stack (libwhisper_ct2, OpenBLAS, CTranslate2, …) is compiled
  locally, so the bundle only runs on distros at least as new as the build
  host (Arch/tumbleweed-class as of June 2026). The old-distro support that
  bundling libstdc++ was meant to buy (Ubuntu 22.04/Debian 12) does NOT
  currently hold — that check only covered GLIBCXX, not glibc itself. To
  lower the floor, rebuild the native stack (go-whisper-ct2 repo) inside an
  old base image (e.g. ubuntu:22.04) and build the bundle from those libs;
  the bundle script measures and reports the floor on every build.
- **No model checksum verification** — `config.ModelInfo.Checksum` is always
  `""` and never checked; downloads are trusted from HuggingFace over HTTPS.
- **Test coverage** — only `internal/config` has tests (a race test for model
  switching). Everything else is manually tested (checklist in CLAUDE.md).
- **Download progress is approximate** — byte-weighted against the model's
  approximate size, so it can hit 100% slightly early. Cosmetic.
- **Windows runtime is untested** — the Go side cross-compiles and the bundle
  script is ready, but gohook hotkeys, beeep notifications, WASAPI capture,
  and the named-pipe IPC have not been exercised on a real Windows machine.
  Auto-hide/keep-hidden are deliberate no-ops on Windows (no compositor hide,
  and Gio v0.9 exposes no minimize action).
- **macOS** — entirely untrodden; the C++ stack has never been built for it.
