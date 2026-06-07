package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Config holds all application settings
type Config struct {
	mu sync.RWMutex `json:"-"`

	// Model settings
	Model          string `json:"model"`            // "tiny", "base", "small", "medium"
	ModelCachePath string `json:"model_cache_path"` // Custom path or empty for default

	// Audio settings
	SampleRate int `json:"sample_rate"` // 16000 (required by Whisper)
	Channels   int `json:"channels"`    // 1 (mono)

	// UI settings
	AutoHide   bool `json:"auto_hide"`   // Hide window when not recording
	KeepHidden bool `json:"keep_hidden"` // Start hidden in tray

	// Transcription settings
	Language   string `json:"language"`    // "en" or empty for auto
	VADEnabled bool   `json:"vad_enabled"` // Voice Activity Detection

	// Internal paths (not saved to JSON)
	configDir    string `json:"-"`
	cacheDir     string `json:"-"`
	dataDir      string `json:"-"`
	vadModelPath string `json:"-"` // path to extracted Silero VAD model
}

// RequiredModelFiles lists the files needed for a valid CTranslate2 model.
// vocabulary.json (or vocabulary.txt) is downloaded but not required for validation
// since some models use different vocabulary file formats.
var RequiredModelFiles = []string{"model.bin", "config.json"}

// ModelInfo contains information about a Whisper model
type ModelInfo struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`         // Approximate expected size in bytes (actual may vary)
	Description string `json:"description"`  // User-friendly description
	DisplayName string `json:"display_name"` // Full display name for UI
	URL         string `json:"url"`          // Download URL
	Checksum    string `json:"checksum"`     // SHA256 checksum
}

// AvailableModels returns information about downloadable models
var AvailableModels = map[string]ModelInfo{
	"tiny": {
		Name:        "tiny",
		Size:        75 * 1024 * 1024, // ~75MB (39M params in FP16)
		Description: "Fastest, multilingual",
		DisplayName: "tiny (~75MB) - Fastest, multilingual",
		URL:         "https://huggingface.co/Systran/faster-whisper-tiny",
		Checksum:    "",
	},
	"base": {
		Name:        "base",
		Size:        145 * 1024 * 1024, // ~145MB (74M params in FP16)
		Description: "Fast, multilingual",
		DisplayName: "base (~145MB) - Fast, multilingual",
		URL:         "https://huggingface.co/Systran/faster-whisper-base",
		Checksum:    "",
	},
	"small": {
		Name:        "small",
		Size:        486 * 1024 * 1024, // ~486MB (244M params in FP16)
		Description: "⭐ Recommended (accurate & fast, multilingual)",
		DisplayName: "small (~486MB) - ⭐ Recommended (accurate & fast, multilingual)",
		URL:         "https://huggingface.co/Systran/faster-whisper-small",
		Checksum:    "",
	},
	"medium": {
		Name:        "medium",
		Size:        1500 * 1024 * 1024, // ~1.5GB (769M params in FP16)
		Description: "Great accuracy, multilingual",
		DisplayName: "medium (~1.5GB) - Great accuracy, multilingual",
		URL:         "https://huggingface.co/Systran/faster-whisper-medium",
		Checksum:    "",
	},
	"large-v3": {
		Name:        "large-v3",
		Size:        3000 * 1024 * 1024, // ~3GB (1550M params in FP16)
		Description: "Best accuracy, multilingual",
		DisplayName: "large-v3 (~3GB) - Best accuracy, multilingual",
		URL:         "https://huggingface.co/Systran/faster-whisper-large-v3",
		Checksum:    "",
	},
}

// Default returns a new Config with default values
func Default() *Config {
	return &Config{
		Model:      "small",
		SampleRate: 16000,
		Channels:   1,
		AutoHide:   false,
		KeepHidden: false,
		Language:   "en",
		VADEnabled: true,
	}
}

// Load loads config from the default path or creates default config
func Load() (*Config, error) {
	cfg := Default()

	// Set platform-specific paths
	if err := cfg.initPaths(); err != nil {
		return nil, fmt.Errorf("failed to initialize paths: %w", err)
	}

	// Try to load existing config
	configFile := cfg.ConfigFilePath()
	data, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file, use defaults and save
			if err := cfg.Save(); err != nil {
				return nil, fmt.Errorf("failed to save default config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse existing config
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate and fix any issues
	cfg.validate()

	return cfg, nil
}

// Save writes the current config to disk
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Ensure config directory exists
	if err := os.MkdirAll(c.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configFile := c.ConfigFilePath()
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// initPaths sets platform-specific directories
func (c *Config) initPaths() error {
	switch runtime.GOOS {
	case "linux":
		// Follow XDG Base Directory Specification
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		// Config: ~/.config/voice-to-clipboard/
		c.configDir = filepath.Join(homeDir, ".config", "voice-to-clipboard")
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			c.configDir = filepath.Join(xdgConfig, "voice-to-clipboard")
		}

		// Cache: ~/.cache/voice-to-clipboard/
		c.cacheDir = filepath.Join(homeDir, ".cache", "voice-to-clipboard")
		if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
			c.cacheDir = filepath.Join(xdgCache, "voice-to-clipboard")
		}

		// Data: ~/.local/share/voice-to-clipboard/
		c.dataDir = filepath.Join(homeDir, ".local", "share", "voice-to-clipboard")
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			c.dataDir = filepath.Join(xdgData, "voice-to-clipboard")
		}

	case "windows":
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		if appData == "" || localAppData == "" {
			return errors.New("could not determine Windows app data directories")
		}

		c.configDir = filepath.Join(appData, "VoiceToClipboard")
		c.cacheDir = filepath.Join(localAppData, "VoiceToClipboard", "cache")
		c.dataDir = filepath.Join(localAppData, "VoiceToClipboard")

	case "darwin":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		c.configDir = filepath.Join(homeDir, "Library", "Application Support", "VoiceToClipboard")
		c.cacheDir = filepath.Join(homeDir, "Library", "Caches", "VoiceToClipboard")
		c.dataDir = filepath.Join(homeDir, "Library", "Application Support", "VoiceToClipboard")

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Create directories if they don't exist
	for _, dir := range []string{c.configDir, c.cacheDir, c.dataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// validate checks and fixes config values
func (c *Config) validate() {
	// Ensure valid model
	if _, ok := AvailableModels[c.Model]; !ok {
		c.Model = "small"
	}

	// Ensure valid sample rate (Whisper requires 16kHz)
	if c.SampleRate != 16000 {
		c.SampleRate = 16000
	}

	// Ensure mono
	if c.Channels != 1 {
		c.Channels = 1
	}

	// Language defaults
	if c.Language == "" {
		c.Language = "en"
	}
}

// ConfigFilePath returns the path to the config file
func (c *Config) ConfigFilePath() string {
	return filepath.Join(c.configDir, "config.json")
}

// ModelCacheDir returns the directory where models are cached
func (c *Config) ModelCacheDir() string {
	if c.ModelCachePath != "" {
		return c.ModelCachePath
	}
	return filepath.Join(c.cacheDir, "models")
}

// ModelPath returns the full path to the current model directory (CTranslate2 format)
func (c *Config) ModelPath() string {
	return filepath.Join(c.ModelCacheDir(), c.Model)
}

// LogFilePath returns the path to the log file
func (c *Config) LogFilePath() string {
	return filepath.Join(c.dataDir, "app.log")
}

// ConfigDir returns the config directory path
func (c *Config) ConfigDir() string {
	return c.configDir
}

// CacheDir returns the cache directory path
func (c *Config) CacheDir() string {
	return c.cacheDir
}

// DataDir returns the data directory path
func (c *Config) DataDir() string {
	return c.dataDir
}

// VADModelPath returns the path to the extracted Silero VAD model (empty if not set).
func (c *Config) VADModelPath() string {
	return c.vadModelPath
}

// SetVADModelPath records the on-disk path of the extracted Silero VAD model.
func (c *Config) SetVADModelPath(path string) {
	c.vadModelPath = path
}

// SetModel changes the current model (thread-safe)
func (c *Config) SetModel(model string) error {
	if _, ok := AvailableModels[model]; !ok {
		return fmt.Errorf("unknown model: %s", model)
	}

	c.mu.Lock()
	c.Model = model
	c.mu.Unlock()

	return c.Save()
}

// GetModel returns the current model name (thread-safe)
func (c *Config) GetModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Model
}

// HasModel checks if the current model exists on disk
func (c *Config) HasModel() bool {
	modelPath := c.ModelPath()
	info, err := os.Stat(modelPath)
	if err != nil {
		return false
	}

	// CTranslate2 models are directories
	if !info.IsDir() {
		return false
	}

	// Check for required model files
	for _, file := range RequiredModelFiles {
		filePath := filepath.Join(modelPath, file)
		if _, err := os.Stat(filePath); err != nil {
			return false
		}
	}

	return true
}

// GetModelInfo returns information about the current model
func (c *Config) GetModelInfo() (ModelInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	info, ok := AvailableModels[c.Model]
	return info, ok
}

// SetAutoHide sets the auto-hide setting (thread-safe, persists to disk)
func (c *Config) SetAutoHide(enabled bool) error {
	c.mu.Lock()
	c.AutoHide = enabled
	c.mu.Unlock()
	return c.Save()
}

// GetAutoHide returns the auto-hide setting (thread-safe)
func (c *Config) GetAutoHide() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoHide
}

// SetKeepHidden sets the keep-hidden setting (thread-safe, persists to disk)
func (c *Config) SetKeepHidden(enabled bool) error {
	c.mu.Lock()
	c.KeepHidden = enabled
	c.mu.Unlock()
	return c.Save()
}

// GetKeepHidden returns the keep-hidden setting (thread-safe)
func (c *Config) GetKeepHidden() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.KeepHidden
}
