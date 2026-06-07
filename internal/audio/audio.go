package audio

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"

	"voice-to-clipboard/internal/config"
	"voice-to-clipboard/internal/logger"
)

// Visualizer bar constants
const (
	VisualizerBarMinHeight = 3
	VisualizerBarMaxHeight = 18
)

// Recorder handles audio capture from the microphone
type Recorder struct {
	config *config.Config

	// malgo resources
	context *malgo.AllocatedContext
	device  *malgo.Device

	// Audio buffer
	buffer   []float32
	bufferMu sync.Mutex

	// State
	isRecording atomic.Bool

	// Channel for audio levels (for visualizer)
	levelChan chan float32
}

// NewRecorder creates a new audio recorder
func NewRecorder(cfg *config.Config) (*Recorder, error) {
	// Initialize malgo context
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}

	r := &Recorder{
		config:    cfg,
		context:   ctx,
		levelChan: make(chan float32, 10), // Buffered channel for levels
	}

	return r, nil
}

// Start begins recording audio
func (r *Recorder) Start() error {
	if r.isRecording.Load() {
		return errors.New("already recording")
	}

	// Reset buffer
	r.bufferMu.Lock()
	r.buffer = make([]float32, 0, r.config.SampleRate*60) // Pre-allocate for 60 seconds
	r.bufferMu.Unlock()

	// Configure device
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatF32
	deviceConfig.Capture.Channels = uint32(r.config.Channels)
	deviceConfig.SampleRate = uint32(r.config.SampleRate)
	deviceConfig.Alsa.NoMMap = 1 // Better compatibility on Linux

	// Audio callback
	onRecvFrames := func(outputSamples, inputSamples []byte, frameCount uint32) {
		if !r.isRecording.Load() {
			return
		}

		// Convert bytes to float32
		samples := bytesToFloat32(inputSamples)

		// Add to buffer
		r.bufferMu.Lock()
		r.buffer = append(r.buffer, samples...)
		r.bufferMu.Unlock()

		// Calculate and send level (non-blocking)
		level := CalculateRMS(samples)
		select {
		case r.levelChan <- level:
		default:
			// Drop level update if channel is full
		}
	}

	// Create capture device
	callbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	device, err := malgo.InitDevice(r.context.Context, deviceConfig, callbacks)
	if err != nil {
		return err
	}

	r.device = device
	r.isRecording.Store(true)

	// Start capture
	if err := r.device.Start(); err != nil {
		r.device.Uninit()
		r.isRecording.Store(false)
		return err
	}

	logger.Debug("Recording device started")
	return nil
}

// Stop stops recording and returns the audio buffer
func (r *Recorder) Stop() ([]float32, error) {
	if !r.isRecording.Load() {
		return nil, errors.New("not recording")
	}

	r.isRecording.Store(false)

	// Stop device
	if r.device != nil {
		r.device.Stop()
		r.device.Uninit()
		r.device = nil
	}

	// Get buffer copy
	r.bufferMu.Lock()
	result := make([]float32, len(r.buffer))
	copy(result, r.buffer)
	r.buffer = nil
	r.bufferMu.Unlock()

	logger.Debug("Recording device stopped", "samples", len(result), "duration_sec", float64(len(result))/float64(r.config.SampleRate))
	return result, nil
}

// LevelChannel returns the channel for audio level updates
func (r *Recorder) LevelChannel() <-chan float32 {
	return r.levelChan
}

// Close cleans up recorder resources
func (r *Recorder) Close() {
	if r.isRecording.Load() {
		r.Stop()
	}

	if r.context != nil {
		r.context.Uninit()
		r.context.Free()
	}

	close(r.levelChan)
}

// bytesToFloat32 converts a byte slice to float32 samples
func bytesToFloat32(data []byte) []float32 {
	if len(data) == 0 {
		return nil
	}

	// Each float32 is 4 bytes
	numSamples := len(data) / 4
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		offset := i * 4
		// Little-endian float32
		bits := uint32(data[offset]) |
			uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 |
			uint32(data[offset+3])<<24
		samples[i] = math.Float32frombits(bits)
	}

	return samples
}

// CalculateRMS calculates the root mean square of audio samples
func CalculateRMS(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}

	return float32(math.Sqrt(sum / float64(len(samples))))
}

// CalculateBarHeights converts an RMS level to visualizer bar heights
func CalculateBarHeights(level float32, numBars int) []int {
	heights := make([]int, numBars)

	// Normalize level - amplify significantly for better visualization
	// Typical speech RMS is 0.01-0.1, we want to see movement at low levels
	normalized := float64(level) * 50.0 // Much more amplification
	if normalized > 1.0 {
		normalized = 1.0
	}

	// Apply a curve for better visual response (less aggressive power)
	normalized = math.Pow(normalized, 0.5)

	for i := 0; i < numBars; i++ {
		// Add random variation for organic feel (like Python version)
		variation := 0.5 + (float64((i*7+int(level*1000))%10) * 0.1)
		h := int(normalized * float64(VisualizerBarMaxHeight) * variation)

		if h < VisualizerBarMinHeight {
			h = VisualizerBarMinHeight
		}
		if h > VisualizerBarMaxHeight {
			h = VisualizerBarMaxHeight
		}
		heights[i] = h
	}

	return heights
}

// SmoothLevel smooths audio levels with fast attack and slow decay
func SmoothLevel(currentLevel, targetLevel float32) float32 {
	if targetLevel > currentLevel {
		return targetLevel // Fast attack
	}
	return currentLevel*0.85 + targetLevel*0.15 // Slow decay
}

// ConvertBarHeightsToFloat converts integer bar heights to normalized float32 values (0-1 range)
func ConvertBarHeightsToFloat(heights []int) []float32 {
	floatHeights := make([]float32, len(heights))
	for i, h := range heights {
		floatHeights[i] = float32(h) / float32(VisualizerBarMaxHeight)
	}
	return floatHeights
}
