package transcribe

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/xPrimeTime/go-whisper-ct2/pkg/whisper"

	"voice-to-clipboard/internal/config"
	"voice-to-clipboard/internal/logger"
)

// TranscribeOptions holds options for transcription
type TranscribeOptions struct {
	Language     string // Language code ("en", "auto", etc.)
	BeamSize     int    // Beam search size (1 = greedy, 5 = default, higher = slower but potentially better)
	VADEnabled   bool   // Use the library's Silero VAD to filter non-speech
	VADModelPath string // Path to silero_vad_v6.onnx (required for Silero VAD)
}

// Result holds the transcription result
type Result struct {
	Text     string
	Language string
	Duration float64
	Error    error
}

// Whisper wraps the CTranslate2 whisper library
type Whisper struct {
	model     *whisper.Model
	modelPath string
	loaded    bool
	mu        sync.Mutex
}

// NewWhisper creates a new Whisper instance
func NewWhisper(cfg *config.Config) (*Whisper, error) {
	return &Whisper{
		modelPath: cfg.ModelPath(),
		loaded:    false,
	}, nil
}

// LoadModel loads the whisper model into memory
func (w *Whisper) LoadModel() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.loaded {
		return nil
	}

	// Extract model name from path for display
	modelName := filepath.Base(w.modelPath)
	logger.Info("Loading model", "name", modelName)

	startTime := time.Now()

	// Configure model for optimal CPU performance (v1.1.0+)
	// Using 'default' allows CTranslate2 to handle auto-conversion from float16 models
	modelConfig := whisper.ModelConfig{
		Device:       "cpu",
		ComputeType:  "default", // int8 is unsupported on this CPU/CT2 build; default -> float32
		InterThreads: 1,         // Batch parallelization (1 for single-file)
		IntraThreads: 0,         // Auto-detect threads (uses OMP_NUM_THREADS if set)
	}

	model, err := whisper.LoadModel(w.modelPath, modelConfig)
	if err != nil {
		logger.Error("Failed to load whisper model", "error", err)
		return err
	}

	loadTime := time.Since(startTime).Seconds()
	w.model = model
	w.loaded = true
	logger.Info("Model loaded", "duration_seconds", loadTime)

	return nil
}

// Transcribe transcribes audio samples to text
func (w *Whisper) Transcribe(audio []float32, opts TranscribeOptions) (Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.loaded || w.model == nil {
		return Result{}, errors.New("model not loaded")
	}

	duration := float64(len(audio)) / 16000.0

	// Build transcribe options
	transcribeOpts := []whisper.Option{
		whisper.WithBeamSize(opts.BeamSize),
	}

	// Set language if specified and not auto
	if opts.Language != "" && opts.Language != "auto" {
		transcribeOpts = append(transcribeOpts, whisper.WithLanguage(opts.Language))
	}

	// Always use transcribe task (translation could be added in future)
	transcribeOpts = append(transcribeOpts, whisper.WithTask("transcribe"))

	// Use the library's Silero VAD to trim non-speech (matches faster-whisper).
	// Falls back to the energy VAD inside the library if the model is missing.
	if opts.VADEnabled {
		transcribeOpts = append(transcribeOpts, whisper.WithVADFilter(true))
		if opts.VADModelPath != "" {
			transcribeOpts = append(transcribeOpts, whisper.WithVADModel(opts.VADModelPath))
		}
	}

	// Transcribe using PCM samples
	startTime := time.Now()
	result, err := w.model.TranscribePCM(audio, transcribeOpts...)
	if err != nil {
		return Result{}, err
	}
	transcribeTime := time.Since(startTime).Seconds()

	// Calculate Real-Time Factor (RTF): processing_time / audio_duration
	// RTF < 1 means faster than real-time, RTF > 1 means slower
	rtf := transcribeTime / duration
	logger.Info("Whisper decode complete", "duration_seconds", transcribeTime, "rtf", rtf)

	return Result{
		Text:     result.Text,
		Language: result.Language,
		Duration: duration,
	}, nil
}

// Unload unloads the model from memory
func (w *Whisper) Unload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.loaded || w.model == nil {
		return
	}

	w.model.Close() // v1.1.0: Close() no longer returns error
	w.model = nil
	w.loaded = false
	logger.Info("Whisper model unloaded")
}

// IsLoaded returns whether a model is currently loaded
func (w *Whisper) IsLoaded() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.loaded
}

// Worker handles background transcription tasks
type Worker struct {
	mu      sync.Mutex
	whisper *Whisper
	cfg     *config.Config
}

// NewWorker creates a new transcription worker
func NewWorker(cfg *config.Config) (*Worker, error) {
	w, err := NewWhisper(cfg)
	if err != nil {
		return nil, err
	}

	return &Worker{
		whisper: w,
		cfg:     cfg,
	}, nil
}

// EnsureModelLoaded loads the model if not already loaded
func (w *Worker) EnsureModelLoaded() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ensureModelLoadedLocked()
}

func (w *Worker) ensureModelLoadedLocked() error {
	// Check if model path matches current config (normalize for comparison)
	expectedPath := filepath.Clean(w.cfg.ModelPath())
	currentPath := filepath.Clean(w.whisper.modelPath)
	if currentPath != expectedPath {
		// Model has changed, need to reload
		logger.Info("Model path changed, reloading", "old", w.whisper.modelPath, "new", expectedPath)
		w.whisper.Unload()
		// Create new whisper instance with updated config
		newWhisper, err := NewWhisper(w.cfg)
		if err != nil {
			return err
		}
		w.whisper = newWhisper
	}

	if !w.whisper.IsLoaded() {
		return w.whisper.LoadModel()
	}
	return nil
}

// Transcribe performs transcription and returns result via channel
func (w *Worker) Transcribe(audioData []float32) <-chan Result {
	resultChan := make(chan Result, 1)

	go func() {
		defer close(resultChan)

		startTime := time.Now()
		audioDuration := float64(len(audioData)) / 16000.0

		logger.Debug("Worker transcribing audio", "duration_seconds", audioDuration)

		// Ensure model is loaded
		w.mu.Lock()
		defer w.mu.Unlock()

		if err := w.ensureModelLoadedLocked(); err != nil {
			resultChan <- Result{Error: err}
			return
		}

		// Perform transcription. Silence filtering is delegated to the library's
		// Silero VAD (configured below) instead of a Go-side energy heuristic.
		opts := TranscribeOptions{
			Language:     w.cfg.GetLanguage(),
			BeamSize:     1, // Greedy decoding for best speed (faster-whisper default for real-time)
			VADEnabled:   w.cfg.GetVADEnabled(),
			VADModelPath: w.cfg.VADModelPath(),
		}

		result, err := w.whisper.Transcribe(audioData, opts)
		if err != nil {
			resultChan <- Result{Error: err}
			return
		}

		logger.Debug("Worker transcription finished", "total_seconds", time.Since(startTime).Seconds())

		resultChan <- result
	}()

	return resultChan
}

// Close cleans up worker resources
func (w *Worker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.whisper != nil {
		w.whisper.Unload()
		w.whisper = nil
	}
}
