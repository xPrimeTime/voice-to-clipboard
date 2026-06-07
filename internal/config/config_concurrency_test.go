package config

import (
	"sync"
	"testing"
)

// TestConfigConcurrentModelAccess exercises the read/write pair that previously
// raced: SetModel writes c.Model under the lock while ModelPath/ModelCacheDir and
// the transcription-path getters read it. Run with -race to verify the locking.
func TestConfigConcurrentModelAccess(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.configDir = dir
	c.cacheDir = dir
	c.dataDir = dir

	models := []string{"tiny", "base", "small"}
	const iterations = 500

	var wg sync.WaitGroup

	// Writers: switch the active model repeatedly (each SetModel also Saves).
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := c.SetModel(models[i%len(models)]); err != nil {
					t.Errorf("SetModel failed: %v", err)
					return
				}
			}
		}()
	}

	// Readers: read everything derived from the locked fields.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = c.ModelPath()
				_ = c.ModelCacheDir()
				_ = c.GetModel()
				_ = c.GetLanguage()
				_ = c.GetVADEnabled()
				_ = c.VADModelPath()
				_ = c.HasModel()
			}
		}()
	}

	// A concurrent VAD-path writer, mirroring startup setting it once but
	// stressing the SetVADModelPath/VADModelPath lock pair.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SetVADModelPath("/tmp/silero_vad_v6.onnx")
		}
	}()

	wg.Wait()
}
