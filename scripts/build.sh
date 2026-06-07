#!/bin/bash
# Build script for voice-to-clipboard with CTranslate2

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# Parse arguments
DEV_MODE=false
for arg in "$@"; do
    case $arg in
        --dev)
            DEV_MODE=true
            shift
            ;;
    esac
done

echo "Building voice-to-clipboard with CTranslate2..."
mkdir -p "$PROJECT_DIR/build/bin"

# Note: Assumes libwhisper_ct2.so is installed system-wide or in LD_LIBRARY_PATH
# If not, install go-whisper-ct2 first:
#   cd /path/to/go-whisper-ct2 && make && sudo make install-cpp

# Build with novulkan tag to avoid Vulkan dependency
go build -tags novulkan -o "$PROJECT_DIR/build/bin/voice-to-clipboard" .

echo ""
echo "Binary built at: $PROJECT_DIR/build/bin/voice-to-clipboard"
ls -lh "$PROJECT_DIR/build/bin/voice-to-clipboard"
echo ""
echo "Dependencies:"
echo "  - CTranslate2 (must be installed system-wide)"
echo "  - libwhisper_ct2.so (from go-whisper-ct2, installed to /usr/local/lib)"

