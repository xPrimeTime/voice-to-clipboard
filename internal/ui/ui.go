// Package ui implements the Voice to Clipboard UI using Gio.
// 120×40dp frameless window with a circular mic button and a 4-bar,
// center-grown audio visualizer.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"voice-to-clipboard/internal/system"
)

// AppName is the application display name
const AppName = "Voice to Clipboard"

// Window dimensions (other layout sizes are applied inline in the layout funcs)
const (
	WindowWidth  = 120
	WindowHeight = 40
)

// Palette
var (
	ColorBackground   = color.NRGBA{R: 0x16, G: 0x16, B: 0x18, A: 0xFF} // near-black, hint of blue
	ColorSurface      = color.NRGBA{R: 0x26, G: 0x26, B: 0x2B, A: 0xFF} // idle button fill
	ColorSurfaceHover = color.NRGBA{R: 0x34, G: 0x34, B: 0x3A, A: 0xFF}
	ColorGlyph        = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xEC, A: 0xFF} // soft white
	ColorWhite        = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	ColorInactive     = color.NRGBA{R: 0x3E, G: 0x3E, B: 0x45, A: 0xFF} // resting bars
	ColorRed          = color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0xFF} // recording
	ColorBlue         = color.NRGBA{R: 0x5C, G: 0x9C, B: 0xF5, A: 0xFF} // transcribing
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

// animSeconds returns wall-clock time in seconds for driving animations.
func animSeconds(gtx layout.Context) float64 {
	return float64(gtx.Now.UnixNano()) / float64(time.Second)
}

// layout draws the entire UI
func (u *UI) layout(gtx layout.Context) layout.Dimensions {
	// Fill background
	paint.FillShape(gtx.Ops, ColorBackground,
		clip.Rect{Max: gtx.Constraints.Max}.Op())

	u.stateMu.RLock()
	state := u.state
	u.stateMu.RUnlock()

	// Recording pulse and transcribing wave are time-driven; keep frames coming.
	if state == StateRecording || state == StateTranscribing {
		gtx.Execute(op.InvalidateCmd{})
	}

	if state == StateDownloading {
		return u.layoutDownloadProgress(gtx)
	}

	return layout.Inset{
		Top:    unit.Dp(6),
		Bottom: unit.Dp(6),
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutMicButton(gtx, state)
			}),
			// Push the visualizer to the right edge
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Min.X}}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.layoutVisualizer(gtx, state)
			}),
		)
	})
}

// layoutMicButton draws the circular mic button with a vector microphone glyph
func (u *UI) layoutMicButton(gtx layout.Context, state State) layout.Dimensions {
	size := gtx.Dp(unit.Dp(28))

	// Determine colors based on state
	var fillColor, glyphColor color.NRGBA
	switch state {
	case StateRecording:
		fillColor = ColorRed
		glyphColor = ColorWhite
	case StateTranscribing:
		fillColor = ColorSurface
		glyphColor = ColorBlue
	default:
		fillColor = ColorSurface
		glyphColor = ColorGlyph
		if u.micBtn.Hovered() {
			fillColor = ColorSurfaceHover
			glyphColor = ColorWhite
		}
	}

	return u.micBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		cx := float32(size) / 2
		cy := float32(size) / 2

		// Pulse ring while recording: expands and fades on a 1.4s cycle.
		// Drawn before the button so it sits underneath, and outside any
		// clip so it can extend past the button bounds.
		if state == StateRecording {
			phase := math.Mod(animSeconds(gtx), 1.4) / 1.4
			grow := float32(phase) * gtx.Metric.PxPerDp * 4
			alpha := uint8(90 * (1 - phase))
			ringR := cx + grow
			ring := ColorRed
			ring.A = alpha
			strokeCircle(gtx.Ops, f32.Pt(cx, cy), ringR, 1.5*gtx.Metric.PxPerDp, ring)
		}

		// Button circle
		circle := clip.Ellipse(image.Rect(0, 0, size, size))
		paint.FillShape(gtx.Ops, fillColor, circle.Op(gtx.Ops))

		u.drawMicGlyph(gtx, glyphColor, cx, cy)

		// Pointer cursor over the button
		area := clip.Rect(image.Rect(0, 0, size, size)).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()

		return layout.Dimensions{Size: image.Point{X: size, Y: size}}
	})
}

// drawMicGlyph draws a microphone (capsule, cradle arc, stem, base) centered
// at (cx, cy). Sizes are in dp, converted via the context's scale factor.
func (u *UI) drawMicGlyph(gtx layout.Context, col color.NRGBA, cx, cy float32) {
	s := gtx.Metric.PxPerDp
	stroke := 1.6 * s

	// Capsule body: 6.5×9dp, sitting above center
	capW := 6.5 * s
	capTop := cy - 7.5*s
	capBot := cy + 1.5*s
	capR := int(capW / 2)
	capsule := clip.RRect{
		Rect: image.Rect(int(cx-capW/2), int(capTop), int(cx+capW/2), int(capBot)),
		NE:   capR, NW: capR, SE: capR, SW: capR,
	}
	paint.FillShape(gtx.Ops, col, capsule.Op(gtx.Ops))

	// Cradle: half-circle arc under the capsule, open side up
	arcR := 5 * s
	arcCy := cy + 0.5*s
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-arcR, arcCy))
	// Sweep from the left point through the bottom to the right point
	p.Arc(f32.Pt(arcR, 0), f32.Pt(arcR, 0), -math.Pi)
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: stroke}.Op())

	// Stem and base
	var stem clip.Path
	stem.Begin(gtx.Ops)
	stem.MoveTo(f32.Pt(cx, arcCy+arcR))
	stem.LineTo(f32.Pt(cx, cy+7.5*s))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: stem.End(), Width: stroke}.Op())

	var base clip.Path
	base.Begin(gtx.Ops)
	base.MoveTo(f32.Pt(cx-3.25*s, cy+7.5*s))
	base.LineTo(f32.Pt(cx+3.25*s, cy+7.5*s))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: base.End(), Width: stroke}.Op())
}

// layoutVisualizer draws the 4 audio level bars, grown from the vertical center
func (u *UI) layoutVisualizer(gtx layout.Context, state State) layout.Dimensions {
	u.barMu.RLock()
	heights := u.barHeights
	u.barMu.RUnlock()

	barWidth := gtx.Dp(unit.Dp(5))
	gap := gtx.Dp(unit.Dp(6))
	minHeight := gtx.Dp(unit.Dp(5))
	maxHeight := gtx.Dp(unit.Dp(24))
	containerHeight := gtx.Dp(unit.Dp(28))

	t := animSeconds(gtx)

	for i := 0; i < 4; i++ {
		level := heights[i]
		barColor := ColorInactive

		switch state {
		case StateRecording:
			// Brightness follows the level
			barColor = lerpColor(ColorInactive, ColorWhite, clamp01((level-0.15)/0.5))
		case StateTranscribing:
			// Levels are gone; run a gentle blue wave instead
			level = float32(0.25 + 0.45*(0.5+0.5*math.Sin(t*6-float64(i)*0.9)))
			barColor = ColorBlue
		default:
			level = 0 // resting dots
		}

		height := minHeight + int(level*float32(maxHeight-minHeight))
		if height < minHeight {
			height = minHeight
		}
		if height > maxHeight {
			height = maxHeight
		}

		x := i * (barWidth + gap)
		y := (containerHeight - height) / 2
		bar := clip.RRect{
			Rect: image.Rect(x, y, x+barWidth, y+height),
			NE:   barWidth / 2, NW: barWidth / 2, SE: barWidth / 2, SW: barWidth / 2,
		}
		paint.FillShape(gtx.Ops, barColor, bar.Op(gtx.Ops))
	}

	width := 4*barWidth + 3*gap
	return layout.Dimensions{Size: image.Point{X: width, Y: containerHeight}}
}

// layoutDownloadProgress draws the download progress bar with percentage
func (u *UI) layoutDownloadProgress(gtx layout.Context) layout.Dimensions {
	u.downloadProgressMu.RLock()
	progress := u.downloadProgress
	u.downloadProgressMu.RUnlock()

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Vertical,
			Alignment: layout.Middle,
		}.Layout(gtx,
			// Progress bar
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				barWidth := gtx.Dp(unit.Dp(96))
				barHeight := gtx.Dp(unit.Dp(6))
				radius := barHeight / 2

				track := clip.RRect{
					Rect: image.Rect(0, 0, barWidth, barHeight),
					NE:   radius, NW: radius, SE: radius, SW: radius,
				}
				paint.FillShape(gtx.Ops, ColorSurface, track.Op(gtx.Ops))

				fillWidth := int(float64(barWidth) * progress)
				if fillWidth > barHeight { // keep the capsule shape intact
					fill := clip.RRect{
						Rect: image.Rect(0, 0, fillWidth, barHeight),
						NE:   radius, NW: radius, SE: radius, SW: radius,
					}
					paint.FillShape(gtx.Ops, ColorBlue, fill.Op(gtx.Ops))
				}

				return layout.Dimensions{Size: image.Point{X: barWidth, Y: barHeight}}
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			// Percentage text
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				percentText := fmt.Sprintf("%d%%", int(progress*100))
				label := material.Label(u.theme, unit.Sp(10), percentText)
				label.Color = ColorGlyph
				label.Alignment = text.Middle
				label.Font.Weight = font.Medium
				return label.Layout(gtx)
			}),
		)
	})
}

// strokeCircle strokes a circle outline centered at c with radius r.
func strokeCircle(ops *op.Ops, c f32.Point, r, width float32, col color.NRGBA) {
	circle := clip.Ellipse(image.Rect(
		int(c.X-r), int(c.Y-r),
		int(c.X+r), int(c.Y+r),
	))
	paint.FillShape(ops, col, clip.Stroke{Path: circle.Path(ops), Width: width}.Op())
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func lerpColor(a, b color.NRGBA, t float32) color.NRGBA {
	return color.NRGBA{
		R: uint8(float32(a.R) + (float32(b.R)-float32(a.R))*t),
		G: uint8(float32(a.G) + (float32(b.G)-float32(a.G))*t),
		B: uint8(float32(a.B) + (float32(b.B)-float32(a.B))*t),
		A: uint8(float32(a.A) + (float32(b.A)-float32(a.A))*t),
	}
}
