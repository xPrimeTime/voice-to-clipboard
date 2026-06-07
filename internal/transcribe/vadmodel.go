package transcribe

import (
	"io/fs"
	"os"
	"path/filepath"

	"voice-to-clipboard/internal/logger"
)

// vadModelName is the bundled Silero VAD model filename.
const vadModelName = "silero_vad_v6.onnx"

// ExtractVADModel writes the embedded Silero VAD model to destDir and returns its
// path. onnxruntime needs a real file path, so the asset (embedded via go:embed)
// is materialized on disk. It is rewritten only when missing or a different size.
func ExtractVADModel(assets fs.FS, destDir string) (string, error) {
	data, err := fs.ReadFile(assets, "assets/"+vadModelName)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	destPath := filepath.Join(destDir, vadModelName)

	if info, err := os.Stat(destPath); err == nil && info.Size() == int64(len(data)) {
		return destPath, nil // already extracted and current
	}

	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return "", err
	}
	logger.Info("Extracted Silero VAD model", "path", destPath)
	return destPath, nil
}
