package tray

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"sync"

	"fyne.io/systray"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"voice-to-clipboard/internal/config"
	"voice-to-clipboard/internal/logger"
	"voice-to-clipboard/internal/ui"
)

//go:embed icon.svg
var iconSVGData []byte

var baseIcon image.Image

// Tray manages the system tray icon and menu
type Tray struct {
	config *config.Config

	// Callbacks
	OnToggleRecording  func()
	OnModelSelect      func(model string) error
	OnQuit             func()
	OnKeepHiddenToggle func(enabled bool)
	OnAutoHideToggle   func(enabled bool)

	// Menu items
	statusItem     *systray.MenuItem
	recordItem     *systray.MenuItem
	autoHideItem   *systray.MenuItem
	keepHiddenItem *systray.MenuItem
	modelHeader    *systray.MenuItem
	modelItems     map[string]*systray.MenuItem
	quitItem       *systray.MenuItem

	// State
	statusMu      sync.RWMutex
	statusText    string
	currentStatus string // "idle", "recording", "transcribing"
	currentModel  string
}

// New creates a new system tray
func New(cfg *config.Config) *Tray {
	return &Tray{
		config:        cfg,
		modelItems:    make(map[string]*systray.MenuItem),
		currentModel:  cfg.GetModel(),
		currentStatus: "idle",
	}
}

// loadBaseIcon loads and renders the SVG icon to an image
func loadBaseIcon() image.Image {
	// Parse SVG
	icon, err := oksvg.ReadIconStream(bytes.NewReader(iconSVGData))
	if err != nil {
		logger.Error("Failed to parse SVG icon", "error", err)
		return nil
	}

	// Set size to 64x64 (system tray size)
	width, height := 64, 64
	icon.SetTarget(0, 0, float64(width), float64(height))

	// Create RGBA image
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	// Rasterize SVG
	scanner := rasterx.NewScannerGV(width, height, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(width, height, scanner)
	icon.Draw(raster, 1.0)

	return rgba
}

// createStatusIcon adds a colored status indicator to the base icon
func createStatusIcon(status string) []byte {
	// Ensure base icon is loaded
	if baseIcon == nil {
		baseIcon = loadBaseIcon()
		if baseIcon == nil {
			return nil
		}
	}

	// Create a copy of the base icon
	bounds := baseIcon.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, baseIcon, bounds.Min, draw.Src)

	// Draw status indicator circle in bottom-right corner
	var indicatorColor color.RGBA
	switch status {
	case "recording":
		indicatorColor = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
	case "transcribing":
		indicatorColor = color.RGBA{R: 255, G: 215, B: 0, A: 255} // Gold/Yellow
	default: // "idle" or "ready"
		indicatorColor = color.RGBA{R: 0, G: 200, B: 0, A: 255} // Green
	}

	// Draw filled circle (bottom-right corner, 52,52 center with 8px radius)
	drawCircle(dst, 52, 52, 8, indicatorColor)
	// Draw white border
	drawCircleOutline(dst, 52, 52, 8, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 2)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		logger.Error("Failed to encode icon", "error", err)
		return nil
	}

	return buf.Bytes()
}

// drawCircle draws a filled circle on an RGBA image
func drawCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

// drawCircleOutline draws a circle outline on an RGBA image
func drawCircleOutline(img *image.RGBA, cx, cy, r int, c color.RGBA, thickness int) {
	for y := cy - r - thickness; y <= cy+r+thickness; y++ {
		for x := cx - r - thickness; x <= cx+r+thickness; x++ {
			dx, dy := x-cx, y-cy
			dist := dx*dx + dy*dy
			if dist <= (r+thickness)*(r+thickness) && dist >= (r-thickness)*(r-thickness) {
				img.Set(x, y, c)
			}
		}
	}
}

// Run starts the system tray (blocks until quit)
func (t *Tray) Run() {
	systray.Run(t.onReady, t.onExit)
}

// onReady is called when systray is ready
func (t *Tray) onReady() {
	// Load base icon and set initial status icon
	baseIcon = loadBaseIcon()
	iconData := createStatusIcon("idle")
	if iconData != nil {
		systray.SetIcon(iconData)
	}
	systray.SetTitle(ui.AppName)

	// Status display (non-clickable)
	t.statusItem = systray.AddMenuItem("Status: Ready", "Current status")
	t.statusItem.Disable()

	systray.AddSeparator()

	// Toggle recording
	t.recordItem = systray.AddMenuItem("Start Recording", "Toggle recording")

	systray.AddSeparator()

	// Auto-hide checkbox - initialize from config
	autoHidePrefix := "☐ "
	if t.config.GetAutoHide() {
		autoHidePrefix = "☑ "
	}
	t.autoHideItem = systray.AddMenuItem(autoHidePrefix+"Auto-hide window", "Hide window after completion")

	// Keep window hidden checkbox - initialize from config
	keepHiddenPrefix := "☐ "
	if t.config.GetKeepHidden() {
		keepHiddenPrefix = "☑ "
	}
	t.keepHiddenItem = systray.AddMenuItem(keepHiddenPrefix+"Keep window hidden", "Don't show window when recording")

	systray.AddSeparator()

	// Model selection list (top-level items for better cross-desktop compatibility)
	t.modelHeader = systray.AddMenuItem("Models", "Available models")
	t.modelHeader.Disable()

	// Dynamically build model list from config, sorted by size
	type modelEntry struct {
		name string
		info config.ModelInfo
	}
	var models []modelEntry
	for name, info := range config.AvailableModels {
		models = append(models, modelEntry{name: name, info: info})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].info.Size < models[j].info.Size
	})

	for _, model := range models {
		prefix := "☐ "
		if model.name == t.currentModel {
			prefix = "☑ "
		}
		item := systray.AddMenuItem(prefix+model.info.DisplayName, fmt.Sprintf("Use %s model", model.name))
		t.modelItems[model.name] = item
	}
	logger.Info("Tray model menu initialized", "count", len(models), "current_model", t.currentModel)

	systray.AddSeparator()

	// Quit
	t.quitItem = systray.AddMenuItem("Quit", "Quit "+ui.AppName)

	// Start event loop
	go t.handleEvents()

	logger.Info("System tray initialized")
}

// onExit is called when systray exits
func (t *Tray) onExit() {
	logger.Info("System tray exiting")
}

// handleEvents processes tray menu events
func (t *Tray) handleEvents() {
	// Start goroutines for each model item to handle clicks dynamically
	for modelName, item := range t.modelItems {
		// Capture modelName in closure
		name := modelName
		menuItem := item
		go func() {
			for range menuItem.ClickedCh {
				logger.Info("Tray model click received", "model", name)
				t.selectModel(name)
			}
		}()
	}

	// Main event loop for non-model items
	for {
		select {
		case <-t.recordItem.ClickedCh:
			if t.OnToggleRecording != nil {
				t.OnToggleRecording()
			}

		case <-t.autoHideItem.ClickedCh:
			newValue := !t.config.GetAutoHide()
			if err := t.config.SetAutoHide(newValue); err != nil {
				logger.Error("Failed to save auto-hide setting", "error", err)
			}
			if newValue {
				t.autoHideItem.SetTitle("☑ Auto-hide window")
				logger.Info("Auto-hide enabled")
			} else {
				t.autoHideItem.SetTitle("☐ Auto-hide window")
				logger.Info("Auto-hide disabled")
			}
			if t.OnAutoHideToggle != nil {
				t.OnAutoHideToggle(newValue)
			}

		case <-t.keepHiddenItem.ClickedCh:
			newValue := !t.config.GetKeepHidden()
			if err := t.config.SetKeepHidden(newValue); err != nil {
				logger.Error("Failed to save keep-hidden setting", "error", err)
			}
			if newValue {
				t.keepHiddenItem.SetTitle("☑ Keep window hidden")
				logger.Info("Keep window hidden enabled")
			} else {
				t.keepHiddenItem.SetTitle("☐ Keep window hidden")
				logger.Info("Keep window hidden disabled")
			}
			if t.OnKeepHiddenToggle != nil {
				t.OnKeepHiddenToggle(newValue)
			}

		case <-t.quitItem.ClickedCh:
			if t.OnQuit != nil {
				t.OnQuit()
			}
			systray.Quit()
			return
		}
	}
}

// selectModel changes the selected model
func (t *Tray) selectModel(model string) {
	oldModel := t.currentModel

	// Update UI immediately so user gets click feedback.
	t.updateModelSelectionUI(model)
	t.currentModel = model

	if t.OnModelSelect != nil {
		if err := t.OnModelSelect(model); err != nil {
			logger.Error("Model selection failed", "model", model, "error", err)
			// Roll back selection in tray to reflect actual active model.
			t.updateModelSelectionUI(oldModel)
			t.currentModel = oldModel
			return
		}
	}

	logger.Info("Model selected from tray", "model", model)
}

func (t *Tray) updateModelSelectionUI(selected string) {
	// Uncheck all and update titles using centralized model info.
	for m, item := range t.modelItems {
		modelInfo := config.AvailableModels[m]
		if m == selected {
			item.SetTitle("☑ " + modelInfo.DisplayName)
		} else {
			item.SetTitle("☐ " + modelInfo.DisplayName)
		}
	}
}

// UpdateStatus updates the status display and icon
func (t *Tray) UpdateStatus(status string) {
	t.statusMu.Lock()
	oldStatus := t.currentStatus
	t.statusText = status

	// Parse status string to determine icon state
	var iconStatus string
	var tooltipIcon string
	if status == "Recording..." {
		iconStatus = "recording"
		tooltipIcon = "●"
	} else if status == "Transcribing..." {
		iconStatus = "transcribing"
		tooltipIcon = "⟳"
	} else {
		iconStatus = "idle"
		tooltipIcon = "✓"
	}
	t.currentStatus = iconStatus
	t.statusMu.Unlock()

	// Update menu item
	if t.statusItem != nil {
		t.statusItem.SetTitle("Status: " + status)
	}

	// Update tooltip with Unicode indicator
	systray.SetTooltip(fmt.Sprintf("%s - %s %s", ui.AppName, tooltipIcon, status))

	// Update icon if status changed
	if oldStatus != iconStatus {
		iconData := createStatusIcon(iconStatus)
		if iconData != nil {
			systray.SetIcon(iconData)
			logger.Debug("Updated tray icon", "status", iconStatus)
		}
	}

	// Update record button text
	if t.recordItem != nil {
		if status == "Recording..." {
			t.recordItem.SetTitle("Stop Recording")
		} else {
			t.recordItem.SetTitle("Start Recording")
		}
	}
}

// IsAutoHideEnabled returns whether auto-hide is enabled
func (t *Tray) IsAutoHideEnabled() bool {
	return t.config.GetAutoHide()
}

// IsKeepHiddenEnabled returns whether keep-hidden is enabled
func (t *Tray) IsKeepHiddenEnabled() bool {
	return t.config.GetKeepHidden()
}

// Quit triggers the tray to quit
func (t *Tray) Quit() {
	systray.Quit()
}
