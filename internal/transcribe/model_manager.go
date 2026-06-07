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

	logger.Info("Downloading model from HuggingFace", "repo", modelInfo.URL, "dest", modelPath)

	// Download required files plus vocabulary (which may be .json or .txt)
	downloadFiles := append([]string{}, config.RequiredModelFiles...)
	downloadFiles = append(downloadFiles, "vocabulary.json")

	totalFiles := len(downloadFiles)
	for i, fileName := range downloadFiles {
		// Build URL: https://huggingface.co/{repo}/resolve/main/{file}
		fileURL := fmt.Sprintf("%s/resolve/main/%s", modelInfo.URL, fileName)

		// Download file
		destPath := filepath.Join(modelPath, fileName)
		logger.Info("Downloading file", "file", fileName, "url", fileURL)

		if err := m.downloadFile(fileURL, destPath, 0, nil); err != nil {
			// vocabulary.json might not exist (some models use vocabulary.txt)
			if fileName == "vocabulary.json" {
				// Try vocabulary.txt instead
				altURL := fmt.Sprintf("%s/resolve/main/vocabulary.txt", modelInfo.URL)
				altPath := filepath.Join(modelPath, "vocabulary.txt")
				logger.Info("Trying alternate vocabulary file", "url", altURL)
				if err := m.downloadFile(altURL, altPath, 0, nil); err != nil {
					return fmt.Errorf("failed to download vocabulary: %w", err)
				}
			} else {
				return fmt.Errorf("failed to download %s: %w", fileName, err)
			}
		}

		// Report progress
		if onProgress != nil {
			progress := float64(i+1) / float64(totalFiles)
			onProgress(progress)
		}
	}

	logger.Info("Model downloaded successfully", "model", modelName, "path", modelPath)
	return nil
}

// downloadFile downloads a file from URL to destPath
func (m *ModelManager) downloadFile(url, destPath string, expectedSize int64, onProgress ProgressCallback) error {
	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Start download
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d for %s", resp.StatusCode, url)
	}

	// Create destination file
	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get content length
	totalSize := resp.ContentLength
	if totalSize <= 0 && expectedSize > 0 {
		totalSize = expectedSize
	}

	// Download with progress reporting
	var downloaded int64
	buffer := make([]byte, 32*1024) // 32KB buffer

	lastProgressUpdate := time.Now()
	const progressUpdateInterval = 100 * time.Millisecond

	for {
		// Read chunk
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Write to file
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)

			// Report progress (throttled)
			if onProgress != nil && totalSize > 0 && time.Since(lastProgressUpdate) >= progressUpdateInterval {
				progress := float64(downloaded) / float64(totalSize)
				if progress > 1.0 {
					progress = 1.0
				}
				onProgress(progress)
				lastProgressUpdate = time.Now()
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}
