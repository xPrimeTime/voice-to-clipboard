// Package ui implements the Voice to Clipboard UI using Gio.
// 120×40dp frameless window with mic button and 4-bar audio visualizer.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"voice-to-clipboard/internal/audio"
	"voice-to-clipboard/internal/system"
)

// AppName is the application display name
const AppName = "Voice to Clipboard"

// UI dimension constants
const (
	// Window dimensions
	WindowWidth  = 120
	WindowHeight = 40

	// Button dimensions
	ButtonWidth  = 36
	ButtonHeight = 28
	BorderRadius = 6
	BorderWidth  = 2

	// Visualizer bar dimensions
	BarWidth   = 8
	BarSpacing = 4
	BarRadius  = 3
)

// Colors matching Python GTK4 CSS
var (
	ColorBackground = color.NRGBA{R: 26, G: 26, B: 26, A: 255}    // #1a1a1a
	ColorWhite      = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // #ffffff
	ColorInactive   = color.NRGBA{R: 68, G: 68, B: 68, A: 255}    // #444444
	ColorBlue       = color.NRGBA{R: 74, G: 144, B: 217, A: 255}  // #4a90d9
	ColorHover      = color.NRGBA{R: 255, G: 255, B: 255, A: 38}  // rgba(255,255,255,0.15)
)

// State represents the current application state
type State int

const (
	StateIdle State = iota
	StateRecording
	StateTranscribing
	StateDownloading
)

// UI holds the Gio UI state
type UI struct {
	window *app.Window
	micBtn widget.Clickable
	theme  *material.Theme

	// State
	state      State
	stateMu    sync.RWMutex
	barHeights [4]float32 // 0.0 to 1.0
	barMu      sync.RWMutex

	// Download progress
	downloadProgress   float64 // 0.0 to 1.0
	downloadProgressMu sync.RWMutex

	// Window visibility
	isHidden bool
	hiddenMu sync.Mutex

	// Callbacks
	OnToggleRecording func()
	OnClose           func()
	OnQuit            func()
}

// New creates a new UI instance
func New() *UI {
	return &UI{
		barHeights: [4]float32{0, 0, 0, 0},
		theme:      material.NewTheme(),
	}
}

// Run starts the UI event loop (blocks)
func (u *UI) Run() error {
	u.window = new(app.Window)
	// Adjusted dimensions for better match
	u.window.Option(
		app.Title(AppName),
		app.Size(unit.Dp(WindowWidth), unit.Dp(WindowHeight)),
		app.MinSize(unit.Dp(WindowWidth), unit.Dp(WindowHeight)),
		app.MaxSize(unit.Dp(WindowWidth), unit.Dp(WindowHeight)),
		app.Decorated(false),
	)

	var ops op.Ops

	for {
		switch e := u.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			u.hiddenMu.Lock()
			hidden := u.isHidden
			u.hiddenMu.Unlock()

			// Only render if not hidden
			if !hidden {
				u.handleInput(gtx)
				u.layout(gtx)
				e.Frame(gtx.Ops)
			} else {
				// When hidden, render nothing (transparent/blank)
				e.Frame(gtx.Ops)
			}
		}
	}
}

// SetState updates the UI state
func (u *UI) SetState(s State) {
	u.stateMu.Lock()
	u.state = s
	u.stateMu.Unlock()
	if u.window != nil {
		u.window.Invalidate()
	}
}

// GetState returns current state
func (u *UI) GetState() State {
	u.stateMu.RLock()
	defer u.stateMu.RUnlock()
	return u.state
}

// SetBarHeights updates the visualizer bar heights (0.0 to 1.0)
func (u *UI) SetBarHeights(heights [4]float32) {
	u.barMu.Lock()
	u.barHeights = heights
	u.barMu.Unlock()
	if u.window != nil {
		u.window.Invalidate()
	}
}

// SetDownloadProgress updates the download progress (0.0 to 1.0)
func (u *UI) SetDownloadProgress(progress float64) {
	u.downloadProgressMu.Lock()
	u.downloadProgress = progress
	u.downloadProgressMu.Unlock()
	if u.window != nil {
		u.window.Invalidate()
	}
}

// Hide hides the window using compositor commands (Hyprland/wmctrl)
func (u *UI) Hide() {
	u.hiddenMu.Lock()
	u.isHidden = true
	u.hiddenMu.Unlock()

	// Try using system compositor commands
	_ = system.HideWindow(AppName)
}

// Show shows the window using compositor commands
func (u *UI) Show() {
	u.hiddenMu.Lock()
	wasHidden := u.isHidden
	u.isHidden = false
	u.hiddenMu.Unlock()

	if wasHidden {
		// Try using system compositor commands
		_ = system.ShowWindow(AppName)
		if u.window != nil {
			u.window.Invalidate()
		}
	}
}

// IsHidden returns whether the window is marked as hidden
func (u *UI) IsHidden() bool {
	u.hiddenMu.Lock()
	defer u.hiddenMu.Unlock()
	return u.isHidden
}

// handleInput processes keyboard and mouse input
func (u *UI) handleInput(gtx layout.Context) {
	// Handle mic button click
	if u.micBtn.Clicked(gtx) {
		if u.OnToggleRecording != nil {
			u.OnToggleRecording()
		}
	}

	// Handle keyboard
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.Name("Q"), Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameSpace, key.NameReturn, key.NameEnter:
				if u.OnToggleRecording != nil {
					u.OnToggleRecording()
				}
			case key.NameEscape:
				if u.OnClose != nil {
					u.OnClose()
				} else if u.OnQuit != nil {
					u.OnQuit()
				}
			case key.Name("Q"):
				if u.OnQuit != nil {
					u.OnQuit()
				}
			}
		}
	}
}

// layout draws the entire UI
func (u *UI) layout(gtx layout.Context) layout.Dimensions {
	// Fill background
	paint.FillShape(gtx.Ops, ColorBackground,
		clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Check if we're in download state
	u.stateMu.RLock()
	state := u.state
	u.stateMu.RUnlock()

	if state == StateDownloading {
		return u.layoutDownloadProgress(gtx)
	}

	// Match Python: 4px vertical, 10px horizontal padding
	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(10),
		Right:  unit.Dp(10),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			// Mic button
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutMicButton(gtx)
			}),
			// Spacer (10px box spacing + 6px visualizer margin = 16px total)
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			// Visualizer bars
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutVisualizer(gtx)
			}),
		)
	})
}

// layoutMicButton draws the rounded rectangle/pill mic button (matches Python design)
func (u *UI) layoutMicButton(gtx layout.Context) layout.Dimensions {
	// Rounded rectangle (not full pill) - wider and less rounded
	width := gtx.Dp(unit.Dp(36))
	height := gtx.Dp(unit.Dp(28))
	borderRadius := float32(gtx.Dp(unit.Dp(6))) // Less rounded - 6px instead of 10px
	borderWidth := gtx.Dp(unit.Dp(2))           // 2px border

	u.stateMu.RLock()
	state := u.state
	u.stateMu.RUnlock()

	// Determine colors based on state
	var fillColor, borderColor color.NRGBA
	switch state {
	case StateRecording:
		fillColor = ColorWhite
		borderColor = ColorWhite
	case StateTranscribing:
		fillColor = ColorBlue
		borderColor = ColorBlue
	default:
		fillColor = ColorBackground
		borderColor = ColorWhite
	}

	// Hover effect (15% white overlay)
	if u.micBtn.Hovered() && state == StateIdle {
		fillColor = color.NRGBA{R: 64, G: 64, B: 64, A: 255} // ~15% lighter than #1a1a1a
	}

	return u.micBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw outer rounded rect (border)
		outerRect := image.Rect(0, 0, width, height)
		drawRoundedRect(gtx.Ops, outerRect, borderRadius, borderColor)

		// Draw inner rounded rect (fill)
		innerRect := image.Rect(borderWidth, borderWidth, width-borderWidth, height-borderWidth)
		innerRadius := borderRadius - float32(borderWidth)
		if innerRadius < 0 {
			innerRadius = 0
		}
		drawRoundedRect(gtx.Ops, innerRect, innerRadius, fillColor)

		return layout.Dimensions{Size: image.Point{X: width, Y: height}}
	})
}

// layoutVisualizer draws the 4 audio level bars
func (u *UI) layoutVisualizer(gtx layout.Context) layout.Dimensions {
	u.barMu.RLock()
	heights := u.barHeights
	u.barMu.RUnlock()

	u.stateMu.RLock()
	state := u.state
	u.stateMu.RUnlock()

	// Wider bars and taller container for better visibility
	barWidth := gtx.Dp(unit.Dp(8))
	minHeight := gtx.Dp(unit.Dp(audio.VisualizerBarMinHeight))
	maxHeight := gtx.Dp(unit.Dp(audio.VisualizerBarMaxHeight))
	containerHeight := gtx.Dp(unit.Dp(28))

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.End, // Bars aligned to bottom of their container
	}.Layout(gtx,
		// First bar with left margin (2px)
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.drawBar(gtx, heights[0], barWidth, minHeight, maxHeight, containerHeight, state)
		}),
		// 4dp spacing between bars (2px right + 2px left margins)
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.drawBar(gtx, heights[1], barWidth, minHeight, maxHeight, containerHeight, state)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.drawBar(gtx, heights[2], barWidth, minHeight, maxHeight, containerHeight, state)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.drawBar(gtx, heights[3], barWidth, minHeight, maxHeight, containerHeight, state)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
	)
}

// drawBar draws a single visualizer bar
func (u *UI) drawBar(gtx layout.Context, level float32, width, minH, maxH, containerH int, state State) layout.Dimensions {
	// Calculate height based on level (0.0 to 1.0)
	height := minH + int(level*float32(maxH-minH))
	if height < minH {
		height = minH
	}
	if height > maxH {
		height = maxH
	}

	// Determine color - white when active (height > 5px), gray otherwise
	barColor := ColorInactive
	if state == StateRecording && level > 0.15 {
		barColor = ColorWhite
	}

	// Draw rounded rectangle (3px radius)
	radius := float32(gtx.Dp(unit.Dp(3)))
	rect := image.Rect(0, 0, width, height)

	// Offset to align to bottom of container
	yOffset := containerH - height
	defer op.Offset(image.Point{X: 0, Y: yOffset}).Push(gtx.Ops).Pop()

	drawRoundedRect(gtx.Ops, rect, radius, barColor)

	return layout.Dimensions{Size: image.Point{X: width, Y: containerH}}
}

// layoutDownloadProgress draws the download progress bar with percentage
func (u *UI) layoutDownloadProgress(gtx layout.Context) layout.Dimensions {
	// Get current progress
	u.downloadProgressMu.RLock()
	progress := u.downloadProgress
	u.downloadProgressMu.RUnlock()

	// Center everything vertically and horizontally
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Vertical,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceEvenly,
		}.Layout(gtx,
			// Progress bar
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Progress bar dimensions
				barWidth := gtx.Dp(unit.Dp(100))
				barHeight := gtx.Dp(unit.Dp(8))

				// Draw background bar (gray)
				bgRect := image.Rect(0, 0, barWidth, barHeight)
				drawRoundedRect(gtx.Ops, bgRect, 4, ColorInactive)

				// Draw progress fill (blue)
				fillWidth := int(float64(barWidth) * progress)
				if fillWidth > 0 {
					fillRect := image.Rect(0, 0, fillWidth, barHeight)
					drawRoundedRect(gtx.Ops, fillRect, 4, ColorBlue)
				}

				return layout.Dimensions{Size: image.Point{X: barWidth, Y: barHeight}}
			}),
			// Spacing
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			// Percentage text
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				percentText := fmt.Sprintf("%d%%", int(progress*100))

				// Create label with white color
				label := material.Label(u.theme, unit.Sp(10), percentText)
				label.Color = ColorWhite
				label.Alignment = text.Middle
				label.Font.Weight = font.Bold

				return label.Layout(gtx)
			}),
		)
	})
}

// drawRoundedRect draws a filled rounded rectangle
func drawRoundedRect(ops *op.Ops, rect image.Rectangle, radius float32, col color.NRGBA) {
	r := int(radius)
	rr := clip.RRect{
		Rect: rect,
		NE:   r,
		NW:   r,
		SE:   r,
		SW:   r,
	}
	paint.FillShape(ops, col, rr.Op(ops))
}
