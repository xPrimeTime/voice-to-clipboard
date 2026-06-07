package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"voice-to-clipboard/internal/audio"
	"voice-to-clipboard/internal/config"
	"voice-to-clipboard/internal/hotkey"
	"voice-to-clipboard/internal/ipc"
	"voice-to-clipboard/internal/logger"
	"voice-to-clipboard/internal/system"
	"voice-to-clipboard/internal/transcribe"
	"voice-to-clipboard/internal/tray"
	"voice-to-clipboard/internal/ui"
)

//go:embed assets
var assetsDir embed.FS

// App holds the application state
type App struct {
	ui       *ui.UI
	tray     *tray.Tray
	config   *config.Config
	recorder *audio.Recorder
	player   *audio.Player
	modelMgr *transcribe.ModelManager
	worker   *transcribe.Worker

	isRecording    atomic.Bool
	isTranscribing atomic.Bool
	stopLevels     chan struct{}
	stopLevelsMu   sync.Mutex

	ipcListener net.Listener
	hotkeyMgr   *hotkey.Manager
}

// sendCommandToExistingInstance sends a single IPC command to a running instance
// and waits for its acknowledgment. Returns false if no instance is reachable.
func sendCommandToExistingInstance(command string) bool {
	conn, err := ipc.Dial()
	if err != nil {
		return false
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(command + "\n")); err != nil {
		return false
	}

	// Wait for acknowledgment
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	return true
}

// dispatchControlCommand sends a control command to a running instance and exits
// the process: 0 if the command was delivered, 1 if no instance is running.
func dispatchControlCommand(command string) {
	if sendCommandToExistingInstance(command) {
		fmt.Printf("Sent %s command to running instance\n", command)
		os.Exit(0)
	}
	fmt.Println("No running instance found")
	os.Exit(1)
}

// startIPCServer starts listening for IPC commands
func (a *App) startIPCServer() error {
	listener, err := ipc.Listen()
	if err != nil {
		return err
	}

	a.ipcListener = listener

	go func() {
		defer listener.Close()
		logger.Info("IPC server accepting connections")
		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Debug("IPC listener closed", "error", err)
				return
			}

			go a.handleIPCConnection(conn)
		}
	}()

	return nil
}

// handleIPCConnection handles incoming IPC commands
func (a *App) handleIPCConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	command := string(buf[:n])
	command = strings.TrimSpace(command)

	switch command {
	case "toggle":
		logger.Info("Received toggle command via IPC")
		a.toggleRecording()
		conn.Write([]byte("ok\n"))
	case "quit":
		logger.Info("Received quit command via IPC")
		// Acknowledge before exiting so the client's Read succeeds. The write
		// reaches the peer's socket buffer before quit() calls os.Exit.
		conn.Write([]byte("ok\n"))
		a.quit("ipc")
	case "show":
		logger.Info("Received show command via IPC")
		if a.ui != nil {
			a.ui.Show()
		}
		conn.Write([]byte("ok\n"))
	case "hide":
		logger.Info("Received hide command via IPC")
		if a.ui != nil {
			a.ui.Hide()
		}
		conn.Write([]byte("ok\n"))
	default:
		conn.Write([]byte("unknown\n"))
	}
}

func main() {
	// Control flags drive a running instance over the IPC socket instead of
	// starting a new window. These form a tray-independent control plane that
	// works on any compositor/session (bind them to compositor keybinds): some
	// tray hosts don't deliver menu clicks, and Wayland has no global hotkeys.
	toggleFlag := flag.Bool("toggle", false, "Toggle recording in the running instance")
	quitFlag := flag.Bool("quit", false, "Quit the running instance")
	showFlag := flag.Bool("show", false, "Show the running instance's window")
	hideFlag := flag.Bool("hide", false, "Hide the running instance's window")
	flag.Parse()

	switch {
	case *toggleFlag:
		dispatchControlCommand("toggle")
	case *quitFlag:
		dispatchControlCommand("quit")
	case *showFlag:
		dispatchControlCommand("show")
	case *hideFlag:
		dispatchControlCommand("hide")
	}

	app := &App{}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
		cfg = config.Default()
	}
	app.config = cfg

	// Initialize logger
	if err := logger.Init(cfg.LogFilePath(), false); err != nil {
		log.Printf("Warning: Failed to init logger: %v", err)
	}
	defer logger.Close()

	logger.Info("App starting", "model", cfg.GetModel())

	// Pin CPU inference to physical cores (unless OMP_NUM_THREADS is set) so the
	// float32 Whisper encoder doesn't oversubscribe SMT siblings. Must run before
	// the first transcription loads the model.
	transcribe.ConfigureCPUThreads()

	// Materialize the embedded Silero VAD model so the transcription library can
	// load it (onnxruntime requires a real file path). Non-fatal: the library
	// falls back to its energy VAD if the model is unavailable.
	if vadPath, err := transcribe.ExtractVADModel(assetsDir, cfg.CacheDir()); err != nil {
		logger.Warn("Failed to extract Silero VAD model; using fallback VAD", "error", err)
	} else {
		cfg.SetVADModelPath(vadPath)
	}

	// Initialize components
	app.modelMgr = transcribe.NewModelManager(cfg)

	if recorder, err := audio.NewRecorder(cfg); err != nil {
		logger.Error("Failed to init recorder", "error", err)
	} else {
		app.recorder = recorder
		defer recorder.Close()
	}

	if player, err := audio.NewPlayer(&assetsDir); err != nil {
		logger.Warn("Failed to init player", "error", err)
	} else {
		app.player = player
		// Note: oto v3 uses runtime.SetFinalizer, no explicit cleanup needed
	}

	if worker, err := transcribe.NewWorker(cfg); err != nil {
		logger.Error("Failed to init transcription worker", "error", err)
	} else {
		app.worker = worker
		defer worker.Close()
	}

	// Create UI early so we can show download progress
	app.ui = ui.New()
	app.ui.OnToggleRecording = app.toggleRecording
	app.ui.OnClose = app.closeWindow
	app.ui.OnQuit = func() {
		app.quit("ui")
	}

	// Check for first run - need model download
	needsDownload := !cfg.HasModel()
	if needsDownload {
		logger.Info("First run - no model available, downloading base model...")

		// Show download UI
		app.ui.SetState(ui.StateDownloading)
		app.ui.SetDownloadProgress(0.0)

		// Download in background goroutine
		downloadDone := make(chan error, 1)
		go func() {
			err := app.modelMgr.DownloadModel("base", func(progress float64) {
				logger.Info("Download progress", "percent", int(progress*100))
				app.ui.SetDownloadProgress(progress)
			})
			downloadDone <- err
		}()

		// Start UI to show progress (this blocks until download is complete)
		// We'll check download status in a separate goroutine
		go func() {
			err := <-downloadDone
			if err != nil {
				logger.Error("Failed to download model", "error", err)
				log.Fatal("Cannot start without a model. Please ensure internet connection.")
			}

			cfg.SetModel("base")
			if app.worker != nil {
				app.worker.EnsureModelLoaded()
			}

			// Return to idle state
			app.ui.SetState(ui.StateIdle)
			logger.Info("Model download complete, ready to use")
		}()
	}

	// Start IPC server for handling --toggle commands
	if err := app.startIPCServer(); err != nil {
		logger.Warn("Failed to start IPC server", "error", err)
	} else {
		defer func() {
			if app.ipcListener != nil {
				app.ipcListener.Close()
				if err := ipc.CleanupStale(); err != nil {
					logger.Warn("Failed to clean up IPC endpoint", "error", err)
				}
			}
		}()
		logger.Info("IPC server started", "endpoint", ipc.Address())
	}

	// Start global hotkey listener if supported
	if hotkey.IsSupported() {
		app.hotkeyMgr = hotkey.New(app.toggleRecording)
		if err := app.hotkeyMgr.Start(); err != nil {
			logger.Warn("Failed to start global hotkey listener", "error", err)
			logger.Info("Tip: Use compositor keybinds or --toggle flag instead")
		} else {
			defer app.hotkeyMgr.Stop()
			logger.Info("Global hotkey registered", "key", "Ctrl+Shift+R")
		}
	} else {
		logger.Info("Global hotkeys not supported on this platform")
		logger.Info("Tip: Use compositor keybinds or --toggle flag")
	}

	// Create and start tray in background
	app.tray = tray.New(cfg)
	app.tray.OnToggleRecording = app.toggleRecording
	app.tray.OnModelSelect = app.changeModel
	app.tray.OnQuit = func() {
		app.quit("tray")
	}
	app.tray.OnKeepHiddenToggle = func(enabled bool) {
		if enabled {
			app.ui.Hide()
		} else {
			app.ui.Show()
		}
	}
	app.tray.OnAutoHideToggle = func(enabled bool) {
		// If disabling auto-hide and window is hidden and not keeping hidden, show it
		if !enabled && !app.tray.IsKeepHiddenEnabled() {
			app.ui.Show()
		}
	}
	// Left-clicking the tray icon brings the window back. This is the universal
	// "recover the UI" gesture for hosts where the right-click menu doesn't work.
	app.tray.OnLeftClick = func() {
		app.ui.Show()
	}
	go app.tray.Run()

	// Run UI (blocks)
	logger.Info("Starting UI")
	if err := app.ui.Run(); err != nil {
		logger.Error("UI error", "error", err)
		log.Fatal(err)
	}
}

func (a *App) quit(source string) {
	logger.Info("Quit requested", "source", source)
	if a.tray != nil {
		a.tray.Quit()
	}
	os.Exit(0)
}

func (a *App) closeWindow() {
	// Respect tray visibility settings: close hides the window when auto-hide
	// or keep-hidden are enabled; otherwise it exits the app.
	if a.tray != nil && (a.tray.IsAutoHideEnabled() || a.tray.IsKeepHiddenEnabled()) {
		logger.Info("Window hidden from close action")
		a.ui.Hide()
		return
	}
	a.quit("window-close")
}

func (a *App) toggleRecording() {
	if a.isTranscribing.Load() {
		return
	}

	if a.isRecording.Load() {
		a.stopRecording()
	} else {
		a.startRecording()
	}
}

func (a *App) startRecording() {
	if a.recorder == nil {
		logger.Error("Recorder not available")
		return
	}

	// Show window unless keep-hidden is enabled
	if a.tray != nil && !a.tray.IsKeepHiddenEnabled() {
		a.ui.Show()
	}

	// Play start sound
	if a.player != nil {
		a.player.PlaySoundAsync(audio.SoundStart)
	}

	if err := a.recorder.Start(); err != nil {
		logger.Error("Failed to start recording", "error", err)
		return
	}

	a.isRecording.Store(true)
	a.ui.SetState(ui.StateRecording)
	if a.tray != nil {
		a.tray.UpdateStatus("Recording...")
	}
	logger.Info("Recording started")

	// Start level updates
	a.stopLevelsMu.Lock()
	a.stopLevels = make(chan struct{})
	a.stopLevelsMu.Unlock()
	go a.updateLevels()
}

func (a *App) stopRecording() {
	if a.recorder == nil {
		return
	}

	// Stop level updates
	a.stopLevelsMu.Lock()
	if a.stopLevels != nil {
		close(a.stopLevels)
		a.stopLevels = nil
	}
	a.stopLevelsMu.Unlock()

	// Stop recording
	audioBuffer, err := a.recorder.Stop()
	if err != nil {
		logger.Error("Failed to stop recording", "error", err)
		return
	}

	a.isRecording.Store(false)

	// Play stop sound
	if a.player != nil {
		a.player.PlaySoundAsync(audio.SoundStop)
	}

	// Reset bars
	a.ui.SetBarHeights([4]float32{0, 0, 0, 0})

	logger.Info("Recording stopped", "samples", len(audioBuffer))

	// Start transcription
	if len(audioBuffer) > 0 {
		go a.transcribe(audioBuffer)
	} else {
		a.ui.SetState(ui.StateIdle)
	}
}

func (a *App) transcribe(audioData []float32) {
	a.isTranscribing.Store(true)
	a.ui.SetState(ui.StateTranscribing)
	if a.tray != nil {
		a.tray.UpdateStatus("Transcribing...")
	}

	logger.Info("Transcribing", "samples", len(audioData), "duration_sec", float64(len(audioData))/16000.0)

	if a.worker == nil {
		logger.Error("Transcription worker not available")
		a.isTranscribing.Store(false)
		a.ui.SetState(ui.StateIdle)
		return
	}

	result := <-a.worker.Transcribe(audioData)

	if result.Error != nil {
		logger.Error("Transcription failed", "error", result.Error)
		system.ShowNotification(ui.AppName, "Transcription failed: "+result.Error.Error())
		a.isTranscribing.Store(false)
		a.ui.SetState(ui.StateIdle)
		if a.tray != nil {
			a.tray.UpdateStatus("Error")
			time.AfterFunc(2*time.Second, func() {
				if a.tray != nil {
					a.tray.UpdateStatus("Ready")
				}
			})
			// Auto-hide on error if enabled
			if a.tray.IsAutoHideEnabled() {
				time.AfterFunc(2*time.Second, func() {
					a.ui.Hide()
				})
			}
		}
		return
	}

	text := result.Text
	if text == "" {
		// No speech detected: don't overwrite the user's clipboard with a
		// placeholder; just notify and fall through to the idle/auto-hide reset.
		logger.Info("No speech detected; leaving clipboard unchanged")
		system.ShowNotification(ui.AppName, "No speech detected")
	} else {
		// Copy to clipboard
		if err := system.CopyToClipboard(text); err != nil {
			logger.Error("Failed to copy to clipboard", "error", err)
		}

		// Show notification
		preview := text
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		system.ShowNotification(ui.AppName, "Copied: "+preview)

		// Play done sound
		if a.player != nil {
			a.player.PlaySoundAsync(audio.SoundDone)
		}
	}

	a.isTranscribing.Store(false)
	a.ui.SetState(ui.StateIdle)
	if a.tray != nil {
		a.tray.UpdateStatus("Ready")
	}

	// Auto-hide or show window based on setting
	if a.tray != nil {
		if a.tray.IsAutoHideEnabled() {
			time.AfterFunc(1*time.Second, func() {
				a.ui.Hide()
			})
		} else if !a.tray.IsKeepHiddenEnabled() {
			// Show window if not keeping it hidden
			a.ui.Show()
		}
	}

	logger.Info("Transcription complete", "length", len(text))
}

func (a *App) changeModel(model string) error {
	logger.Info("Changing model", "model", model)

	// Check if model exists
	oldModel := a.config.GetModel()
	if err := a.config.SetModel(model); err != nil {
		logger.Error("Failed to set model", "error", err)
		system.ShowNotification(ui.AppName, "Invalid model: "+model)
		return err
	}

	if !a.config.HasModel() {
		logger.Info("Model not found, downloading", "model", model)

		// Show window for download progress
		a.ui.Show()

		// Show download progress in UI
		a.ui.SetState(ui.StateDownloading)
		a.ui.SetDownloadProgress(0.0)

		if err := a.modelMgr.DownloadModel(model, func(progress float64) {
			logger.Info("Download progress", "model", model, "percent", int(progress*100))
			a.ui.SetDownloadProgress(progress)
		}); err != nil {
			logger.Error("Failed to download model", "error", err)
			system.ShowNotification(ui.AppName, "Failed to download model: "+model)
			// Revert to old model
			a.config.SetModel(oldModel)
			a.ui.SetState(ui.StateIdle)
			return err
		}

		// Download complete, return to idle state
		a.ui.SetState(ui.StateIdle)
	}

	// Reload worker with new model
	if a.worker != nil {
		// EnsureModelLoaded will detect the path change and reload
		if err := a.worker.EnsureModelLoaded(); err != nil {
			logger.Error("Failed to load new model", "error", err)
			system.ShowNotification(ui.AppName, "Failed to load model: "+model)
			// Revert to old model
			a.config.SetModel(oldModel)
			return err
		}
	}

	system.ShowNotification(ui.AppName, "Model changed to: "+model)
	return nil
}

func (a *App) updateLevels() {
	ticker := time.NewTicker(30 * time.Millisecond) // Faster updates for reactivity
	defer ticker.Stop()

	levelChan := a.recorder.LevelChannel()

	// Get a copy of the stop channel under lock
	a.stopLevelsMu.Lock()
	stopChan := a.stopLevels
	a.stopLevelsMu.Unlock()

	var lastLevel float32
	var smoothedLevel float32

	for {
		select {
		case <-stopChan:
			return
		case level := <-levelChan:
			lastLevel = level
		case <-ticker.C:
			if a.isRecording.Load() {
				// Smooth the level using audio module function
				smoothedLevel = audio.SmoothLevel(smoothedLevel, lastLevel)

				heights := audio.CalculateBarHeights(smoothedLevel, 4)
				// Convert int heights to float32 (0-1 range)
				floatHeights := audio.ConvertBarHeightsToFloat(heights)
				var heightsArray [4]float32
				copy(heightsArray[:], floatHeights)
				a.ui.SetBarHeights(heightsArray)
			}
		}
	}
}
