package audio

import (
	"embed"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"voice-to-clipboard/internal/logger"
)

// Player handles audio playback for sound effects
type Player struct {
	context *oto.Context
	mu      sync.Mutex
	ready   bool
	assets  *embed.FS
}

// Sound names
const (
	SoundStart = "start"
	SoundStop  = "stop"
	SoundDone  = "done"
)

// NewPlayer creates a new audio player
func NewPlayer(assets *embed.FS) (*Player, error) {
	// Initialize oto context with standard options
	op := &oto.NewContextOptions{
		SampleRate:   44100,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	}

	ctx, readyChan, err := oto.NewContext(op)
	if err != nil {
		return nil, err
	}

	p := &Player{
		context: ctx,
		ready:   false,
		assets:  assets,
	}

	// Wait for context to be ready in background
	go func() {
		<-readyChan
		p.mu.Lock()
		p.ready = true
		p.mu.Unlock()
		logger.Debug("Audio player ready")
	}()

	return p, nil
}

// PlaySound plays a sound effect by name (blocking)
func (p *Player) PlaySound(name string) error {
	p.mu.Lock()
	if !p.ready {
		p.mu.Unlock()
		return errors.New("audio player not ready")
	}
	p.mu.Unlock()

	if p.assets == nil {
		return errors.New("no asset loader configured")
	}

	// Load sound from assets
	soundPath := "assets/" + name + ".wav"
	data, err := p.assets.ReadFile(soundPath)
	if err != nil {
		logger.Warn("Sound file not found", "path", soundPath, "error", err)
		return err
	}

	// Decode WAV
	decoder, err := NewWavDecoder(data)
	if err != nil {
		logger.Warn("Failed to decode WAV", "path", soundPath, "error", err)
		return err
	}

	// Create player
	player := p.context.NewPlayer(decoder)
	defer player.Close()

	// Play and wait for completion
	player.Play()

	// Wait for playback to complete
	for player.IsPlaying() {
		time.Sleep(10 * time.Millisecond) // Sleep to prevent busy-waiting
	}

	return nil
}

// PlaySoundAsync plays a sound effect in the background (non-blocking)
func (p *Player) PlaySoundAsync(name string) {
	go func() {
		if err := p.PlaySound(name); err != nil {
			logger.Debug("Failed to play sound", "name", name, "error", err)
		}
	}()
}

// WavDecoder is a simple WAV file decoder for embedded sound assets.
// Note: The decoder assumes WAV files match the player's format (44.1kHz, stereo, 16-bit).
// Format fields are parsed for potential future validation but not currently enforced.
type WavDecoder struct {
	data []byte
	pos  int
}

// NewWavDecoder creates a decoder for WAV data
func NewWavDecoder(data []byte) (*WavDecoder, error) {
	if len(data) < 44 {
		return nil, errors.New("data too short for WAV header")
	}

	// Parse WAV header (simplified)
	// RIFF header
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("not a valid WAV file")
	}

	// Find fmt chunk
	fmtOffset := 12
	for fmtOffset < len(data)-8 {
		chunkID := string(data[fmtOffset : fmtOffset+4])
		chunkSize := int(data[fmtOffset+4]) |
			int(data[fmtOffset+5])<<8 |
			int(data[fmtOffset+6])<<16 |
			int(data[fmtOffset+7])<<24

		if chunkID == "fmt " {
			// Find data chunk (skip format parsing since we assume compatible format)
			dataOffset := fmtOffset + 8 + chunkSize
			for dataOffset < len(data)-8 {
				if string(data[dataOffset:dataOffset+4]) == "data" {
					// Found data chunk
					audioStart := dataOffset + 8
					return &WavDecoder{
						data: data[audioStart:],
						pos:  0,
					}, nil
				}
				// Skip to next chunk
				size := int(data[dataOffset+4]) |
					int(data[dataOffset+5])<<8 |
					int(data[dataOffset+6])<<16 |
					int(data[dataOffset+7])<<24
				dataOffset += 8 + size
			}
		}
		fmtOffset += 8 + chunkSize
	}

	return nil, errors.New("could not parse WAV file")
}

// Read implements io.Reader for WavDecoder
func (d *WavDecoder) Read(p []byte) (n int, err error) {
	if d.pos >= len(d.data) {
		return 0, io.EOF
	}

	n = copy(p, d.data[d.pos:])
	d.pos += n
	return n, nil
}
