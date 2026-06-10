param(
    [string]$Version = "dev",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
Set-Location $ProjectDir

$StageRoot = Join-Path $ProjectDir "build\bundle-windows-amd64"
$AppDir = Join-Path $StageRoot "voice-to-clipboard"
$ArchiveName = "voice-to-clipboard-$Version-windows-amd64.zip"
$ArchivePath = Join-Path $ProjectDir (Join-Path $OutputDir $ArchiveName)

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

Require-Command go

if (Test-Path $StageRoot) {
    Remove-Item -Recurse -Force $StageRoot
}
New-Item -ItemType Directory -Path $AppDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $ProjectDir $OutputDir) -Force | Out-Null

Write-Host "[1/5] Building Windows binary" -ForegroundColor Yellow
$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
# -H=windowsgui: GUI subsystem, otherwise every launch opens a console window
go build -tags novulkan -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$Version" -o (Join-Path $AppDir "voice-to-clipboard.exe") .

$searchDirs = New-Object System.Collections.Generic.List[string]
function Add-SearchDir {
    param([string]$Dir)
    if ([string]::IsNullOrWhiteSpace($Dir)) { return }
    if (Test-Path $Dir) { $searchDirs.Add((Resolve-Path $Dir).Path) }
}

Add-SearchDir ([Environment]::GetEnvironmentVariable("WHISPER_CT2_LIB_DIR"))
Add-SearchDir ([Environment]::GetEnvironmentVariable("CT2_LIB_DIR"))

$pathValue = [Environment]::GetEnvironmentVariable("PATH")
if ($pathValue) {
    $pathValue.Split(';') | ForEach-Object { Add-SearchDir $_ }
}

try {
    $ct2Root = python -c "import os, ctranslate2; print(os.path.dirname(ctranslate2.__file__))" 2>$null
    if ($ct2Root) {
        Add-SearchDir $ct2Root.Trim()
        Add-SearchDir (Join-Path $ct2Root.Trim() "lib")
    }
} catch {
    # Optional fallback only; required files are validated below.
}

function Find-FirstFile {
    param([string[]]$Patterns)
    foreach ($dir in $searchDirs) {
        foreach ($pattern in $Patterns) {
            $match = Get-ChildItem -Path $dir -Filter $pattern -File -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($match) { return $match.FullName }
        }
    }
    return $null
}

function Copy-DllFamily {
    param([string]$Path)
    $dir = Split-Path -Parent $Path
    $name = Split-Path -Leaf $Path
    $prefix = ($name -replace '\.dll.*$', '')
    Get-ChildItem -Path $dir -Filter "$prefix*.dll*" -File -ErrorAction SilentlyContinue |
        ForEach-Object {
            Copy-Item $_.FullName -Destination $AppDir -Force
        }
}

Write-Host "[2/5] Locating required runtime DLLs" -ForegroundColor Yellow
$whisperDll = Find-FirstFile -Patterns @("libwhisper_ct2.dll", "whisper_ct2.dll", "*whisper*ct2*.dll")
$ct2Dll = Find-FirstFile -Patterns @("ctranslate2.dll")

if (-not $whisperDll) {
    throw "Missing required runtime: libwhisper_ct2.dll (or whisper_ct2.dll). Set WHISPER_CT2_LIB_DIR to the library directory."
}
if (-not $ct2Dll) {
    throw "Missing required runtime: ctranslate2.dll. Set CT2_LIB_DIR to the library directory."
}

Write-Host "[3/5] Copying runtime DLLs" -ForegroundColor Yellow
Copy-DllFamily -Path $whisperDll
Copy-DllFamily -Path $ct2Dll

$optionalPatterns = @("onnxruntime*.dll", "mkl*.dll", "dnnl*.dll", "libiomp*.dll", "vcomp*.dll", "libomp*.dll", "libgomp*.dll", "openblas*.dll", "libopenblas*.dll", "tbb*.dll")
foreach ($dir in $searchDirs) {
    foreach ($pattern in $optionalPatterns) {
        Get-ChildItem -Path $dir -Filter $pattern -File -ErrorAction SilentlyContinue |
            ForEach-Object {
                Copy-Item $_.FullName -Destination $AppDir -Force
            }
    }
}

if (-not (Test-Path (Join-Path $AppDir "ctranslate2.dll"))) {
    throw "Bundle missing ctranslate2.dll after copy."
}

$whisperCopied = Get-ChildItem -Path $AppDir -Filter "*whisper*ct2*.dll" -File -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $whisperCopied) {
    throw "Bundle missing whisper CT2 runtime DLL after copy."
}

Write-Host "[4/5] Writing portable launcher + notices" -ForegroundColor Yellow
@"
@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
set "APPDATA=%SCRIPT_DIR%data\\appdata"
set "LOCALAPPDATA=%SCRIPT_DIR%data\\localappdata"
if not exist "%APPDATA%" mkdir "%APPDATA%"
if not exist "%LOCALAPPDATA%" mkdir "%LOCALAPPDATA%"
"%SCRIPT_DIR%voice-to-clipboard.exe" %*
endlocal
"@ | Out-File -Encoding ASCII -FilePath (Join-Path $AppDir "run-portable.bat")

@"
Voice to Clipboard bundles or links the following third-party software.
Each component remains under its own license; the URLs lead to source code
and full license texts.

Bundled native libraries (DLLs):
- go-whisper-ct2 (MIT) - https://github.com/xPrimeTime/go-whisper-ct2
- CTranslate2 (MIT) - https://github.com/OpenNMT/CTranslate2
- ONNX Runtime (MIT) - https://github.com/microsoft/onnxruntime
- BLAS/OpenMP backend libraries as packaged with CTranslate2 (e.g.
  OpenBLAS BSD-3-Clause, oneAPI MKL ISSL, libgomp/vcomp runtimes)
- If present: libsndfile (LGPL-2.1), libsamplerate (BSD-2-Clause) and
  codec libraries (FLAC/ogg/vorbis/opus BSD; mpg123/LAME LGPL)

Compiled into the application binary (Go modules):
- Gio UI (UNLICENSE or MIT) - https://gioui.org
- malgo (Unlicense) - https://github.com/gen2brain/malgo
- oto (Apache-2.0) - https://github.com/ebitengine/oto
- beeep (BSD-3-Clause) - https://github.com/gen2brain/beeep
- clipboard (BSD-3-Clause) - https://github.com/atotto/clipboard
- systray (Apache-2.0) - https://github.com/fyne-io/systray
- go-winio (MIT) - https://github.com/microsoft/go-winio
- gohook (MIT) - https://github.com/robotn/gohook

Bundled model:
- Silero VAD v6 (MIT) - https://github.com/snakers4/silero-vad

LGPL components, where present, are dynamically linked as separate DLLs
and can be replaced by the user.
"@ | Out-File -Encoding UTF8 -FilePath (Join-Path $AppDir "THIRD_PARTY_NOTICES.txt")

@"
# Voice to Clipboard (Windows Bundle)

## Run
- Standard mode: double-click `voice-to-clipboard.exe`
- Portable mode: run `run-portable.bat` (stores config/cache under `data/`)

## Notes
- On first run without a model, the app auto-downloads `base`.
- Keep all DLLs in the same folder as the EXE.
"@ | Out-File -Encoding UTF8 -FilePath (Join-Path $AppDir "README.txt")

if (Test-Path (Join-Path $ProjectDir "LICENSE")) {
    Copy-Item (Join-Path $ProjectDir "LICENSE") (Join-Path $AppDir "LICENSE.txt") -Force
}

Write-Host "[5/5] Creating zip + checksum" -ForegroundColor Yellow
if (Test-Path $ArchivePath) {
    Remove-Item $ArchivePath -Force
}
Compress-Archive -Path $AppDir -DestinationPath $ArchivePath -CompressionLevel Optimal

$hash = Get-FileHash -Path $ArchivePath -Algorithm SHA256
"$($hash.Hash.ToLower())  $ArchiveName" | Out-File -Encoding ASCII -FilePath "$ArchivePath.sha256"

$sizeMB = [math]::Round((Get-Item $ArchivePath).Length / 1MB, 2)
Write-Host "Bundle created: $ArchivePath" -ForegroundColor Green
Write-Host "Checksum: $ArchivePath.sha256" -ForegroundColor Green
Write-Host "Size: $sizeMB MB" -ForegroundColor Green
