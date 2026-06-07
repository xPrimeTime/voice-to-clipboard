package transcribe

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"voice-to-clipboard/internal/logger"
)

// ConfigureCPUThreads sets OMP_NUM_THREADS to the physical core count when the
// user has not set it. CTranslate2's float32 CPU kernels run fastest on physical
// cores; the default (auto) oversubscribes SMT siblings, which measured ~35%
// slower on the Whisper encoder. No-op when OMP_NUM_THREADS is already set, so a
// user override is always respected. Must be called before the first
// transcription, i.e. before the OpenMP runtime first reads the variable.
func ConfigureCPUThreads() {
	if os.Getenv("OMP_NUM_THREADS") != "" {
		return
	}
	n := physicalCores()
	if n < 1 {
		return
	}
	if err := os.Setenv("OMP_NUM_THREADS", strconv.Itoa(n)); err != nil {
		logger.Warn("Failed to set OMP_NUM_THREADS", "error", err)
		return
	}
	logger.Info("Configured CPU threads for transcription", "OMP_NUM_THREADS", n)
}

// physicalCores returns the number of physical CPU cores by counting unique
// (physical id, core id) pairs in /proc/cpuinfo. It falls back to
// runtime.NumCPU() when that file is unavailable (non-Linux) or unparseable.
func physicalCores() int {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.NumCPU()
	}

	type coreKey struct{ pkg, core string }
	seen := make(map[coreKey]struct{})
	var pkg string
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "physical id":
			pkg = value
		case "core id":
			seen[coreKey{pkg: pkg, core: value}] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return runtime.NumCPU()
	}
	return len(seen)
}
