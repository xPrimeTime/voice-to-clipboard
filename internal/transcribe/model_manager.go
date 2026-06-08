package transcribe

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"voice-to-clipboard/internal/config"
	"voice-to-clipboard/internal/logger"
)

// ProgressCallback is called with download progress (0.0 to 1.0)
type ProgressCallback func(progress float64)

// ModelManager handles model downloads and verification
type ModelManager struct {
	config     *config.Config
	httpClient *http.Client

	// Download state
	mu          sync.Mutex
	downloading bool
}

// NewModelManager creates a new model manager
func NewModelManager(cfg *config.Config) *ModelManager {
	return &ModelManager{
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
		config: cfg,
	}
}

// DownloadModel downloads a CTranslate2 model from HuggingFace via direct HTTP
func (m *ModelManager) DownloadModel(modelName string, onProgress ProgressCallback) error {
	m.mu.Lock()
	if m.downloading {
		m.mu.Unlock()
		return errors.New("another download is already in progress")
	}
	m.downloading = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.downloading = false
		m.mu.Unlock()
	}()

	// Get model info
	modelInfo, ok := config.AvailableModels[modelName]
	if !ok {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	// Ensure cache directory exists
	cacheDir := m.config.ModelCacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download destination
	modelPath := filepath.Join(cacheDir, modelName)
	if err := os.MkdirAll(modelPath, 0755); err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}

	// Remove the partial download on failure so retries start clean and an
	// incomplete dir is never mistaken for a usable model. DownloadModel is only
	// called when the model isn't already valid, so this can't delete a good one.
	success := false
	defer func() {
		if !success {
			if err := os.RemoveAll(modelPath); err != nil {
				logger.Warn("Failed to clean up partial model download", "path", modelPath, "error", err)
			}
		}
	}()

	logger.Info("Downloading model from HuggingFace", "repo", modelInfo.URL, "dest", modelPath)

	// Download required files plus vocabulary (which may be .json or .txt)
	downloadFiles := append([]string{}, config.RequiredModelFiles...)
	downloadFiles = append(downloadFiles, "vocabulary.json")

	// Byte-weighted progress: model.bin is ~99% of the bytes, so counting files
	// equally makes the bar lurch. Accumulate downloaded bytes against the
	// model's approximate total size instead, throttled to ~10 updates/sec.
	var downloaded int64
	lastUpdate := time.Now()
	report := func(force bool) {
		if onProgress == nil || modelInfo.Size <= 0 {
			return
		}
		if !force && time.Since(lastUpdate) < 100*time.Millisecond {
			return
		}
		progress := float64(downloaded) / float64(modelInfo.Size)
		if progress > 1.0 {
			progress = 1.0
		}
		onProgress(progress)
		lastUpdate = time.Now()
	}
	onDelta := func(n int) {
		downloaded += int64(n)
		report(false)
	}

	for _, fileName := range downloadFiles {
		// Build URL: https://huggingface.co/{repo}/resolve/main/{file}
		fileURL := fmt.Sprintf("%s/resolve/main/%s", modelInfo.URL, fileName)

		// Download file
		destPath := filepath.Join(modelPath, fileName)
		logger.Info("Downloading file", "file", fileName, "url", fileURL)

		if err := m.downloadFile(fileURL, destPath, onDelta); err != nil {
			// vocabulary.json might not exist (some models use vocabulary.txt)
			if fileName == "vocabulary.json" {
				// Try vocabulary.txt instead
				altURL := fmt.Sprintf("%s/resolve/main/vocabulary.txt", modelInfo.URL)
				altPath := filepath.Join(modelPath, "vocabulary.txt")
				logger.Info("Trying alternate vocabulary file", "url", altURL)
				if err := m.downloadFile(altURL, altPath, onDelta); err != nil {
					return fmt.Errorf("failed to download vocabulary: %w", err)
				}
			} else {
				return fmt.Errorf("failed to download %s: %w", fileName, err)
			}
		}
	}

	// Size is approximate, so force the bar to 100% on success.
	if onProgress != nil {
		onProgress(1.0)
	}

	success = true
	logger.Info("Model downloaded successfully", "model", modelName, "path", modelPath)
	return nil
}

// downloadFile streams url to destPath, invoking onDelta (if non-nil) with the
// byte count of each chunk written so the caller can track overall progress.
func (m *ModelManager) downloadFile(url, destPath string, onDelta func(n int)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d for %s", resp.StatusCode, url)
	}

	// Stream to a temp file and rename on success, so an interrupted download
	// never leaves a partial file that looks complete.
	tmpPath := destPath + ".part"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath) // harmless no-op once renamed

	buffer := make([]byte, 32*1024) // 32KB buffer
	writeErr := func() error {
		defer file.Close()
		for {
			n, rerr := resp.Body.Read(buffer)
			if n > 0 {
				if _, werr := file.Write(buffer[:n]); werr != nil {
					return werr
				}
				if onDelta != nil {
					onDelta(n)
				}
			}
			if rerr == io.EOF {
				return nil
			}
			if rerr != nil {
				return rerr
			}
		}
	}()
	if writeErr != nil {
		return writeErr
	}

	return os.Rename(tmpPath, destPath)
}
