package screen

import (
	"fmt"
	"os"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const (
	// UIMargin is the space in pixels between screen border and UI text.
	UIMargin = 2
)

// UI structure to manage user commands and overlay.
type UI struct {
	Enabled bool

	message string // Temporary test on timer
	text    string // Permanent text

	texture  *sdl.Texture
	renderer *sdl.Renderer

	font     *ttf.Font
	fontZoom uint

	fg sdl.Color // TODO: make it configurable
	bg sdl.Color // TODO: make it configurable

	msgTimer *time.Timer
}

// Return a UI instance given a renderer to create the overlay texture.
func NewUI(renderer *sdl.Renderer, zoom uint) *UI {
	font, err := ttf.OpenFont("assets/ui.ttf", int(8*zoom)) // FIXME: make zoom configurable
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open font: %s\n", err)
		return nil // TODO: result, err
	}

	texture, err := renderer.CreateTexture(
		sdl.PIXELFORMAT_RGBA8888,
		sdl.TEXTUREACCESS_TARGET,
		ScreenWidth*int32(zoom),
		ScreenHeight*int32(zoom))
	if err != nil {
		font.Close()
		fmt.Fprintf(os.Stderr, "Failed to create UI texture: %s\n", err)
		return nil // TODO: result, err
	}

	// Scale font up with screen size.
	fontZoom := zoom // TODO: smaller fontZoom for higher zoom.

	// Background transparency.
	texture.SetBlendMode(sdl.BLENDMODE_BLEND)

	ui := UI{
		Enabled:  true,
		texture:  texture,
		renderer: renderer,
		font:     font,
		fontZoom: fontZoom,
		fg:       sdl.Color{R: 0, G: 0, B: 0, A: 0xff},
		bg:       sdl.Color{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	}
	return &ui
}

// Enable turns on the UI overlay.
func (u *UI) Enable() {
	u.Enabled = true
}

// Disable turns off the UI overlay.
func (u *UI) Disable() {
	u.Enabled = false
}

// Refresh UI texture with permanent text and current message (if any).
func (u *UI) repaint() {
	// Reset texture. FIXME: can we do without the background texture altogether?
	u.renderer.SetRenderTarget(u.texture)
	u.renderer.SetDrawColor(0, 0, 0, 0)
	u.renderer.Clear()

	row := 1
	if u.text != "" {
		u.renderText(u.text, row)
		row++
	}

	// TODO: stack messages
	if u.message != "" {
		u.renderText(u.message, row)
	}

	// Disable if there's nothing to display.
	u.Enabled = u.text != "" || u.message != ""

	u.renderer.SetRenderTarget(nil)
}

// Refresh UI texture with permanent text and current message (if any).
func (u *UI) renderText(text string, row int) {
	// Instantiate text with an outline effect. There's probably an easier way.
	u.font.SetOutline(int(u.fontZoom))
	outline, _ := u.font.RenderUTF8Solid(text, u.bg)
	u.font.SetOutline(0)
	msg, _ := u.font.RenderUTF8Solid(text, u.fg)

	// Position vertically. Bottom row is row number 1.
	_, _, _, h, _ := u.texture.Query()
	y := h - int32(u.font.Height()*row) - UIMargin // TODO: FontSize config

	outlineTexture, _ := u.renderer.CreateTextureFromSurface(outline)
	u.renderer.Copy(outlineTexture, nil, &sdl.Rect{X: UIMargin, Y: y - int32(u.fontZoom), W: outline.W, H: outline.H})

	msgTexture, _ := u.renderer.CreateTextureFromSurface(msg)
	u.renderer.Copy(msgTexture, nil, &sdl.Rect{X: UIMargin + int32(u.fontZoom), Y: y, W: msg.W, H: msg.H})
}

// Set permanent text (useful for persistent UI). Call with empty string to
// clear.
func (u *UI) Text(text string) {
	u.text = text
	u.repaint()
}

// Clear temporary message and repaint texture.
func (u *UI) clearMessage() {
	// Make sure to execute in the UI thread in case we were called from a
	// timer thread. TODO: sdl.Do()
	u.message = ""
	sdl.Do(u.repaint)
}

// Message creates a new UI texture with the given message, enables UI and
// starts a timer that will hide the UI when it's done. Takes a text string and
// a duration (in seconds).
func (u *UI) Message(text string, duration time.Duration) {
	// Stop reset timer, a new one will be started.
	// TODO: stack messages
	if u.msgTimer != nil {
		u.msgTimer.Stop()
	}
	u.message = text
	u.msgTimer = time.AfterFunc(time.Second*duration, u.clearMessage)
	u.repaint()
}
