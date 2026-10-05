package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/daemon"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/platform"
)

//go:embed icons/app.png
var appIcon []byte

//go:embed icons/tray.png
var trayGlyph []byte

// The tray's colours: connected and receiving, something in progress, and
// WhatsApp disconnected or sync stopped. Saturated mid-tones read on a light
// menu bar and on a dark one.
var toneColours = map[string]color.NRGBA{
	"ok":   {0x1f, 0xa8, 0x5c, 0xff},
	"warn": {0xe0, 0x9a, 0x00, 0xff},
	"fail": {0xe5, 0x48, 0x4d, 0xff},
}

type tray struct {
	app       *App
	icon      *application.SystemTray
	menu      *application.Menu
	status    *application.MenuItem
	update    *application.MenuItem
	autostart *application.MenuItem

	mu    sync.Mutex
	tone  string
	icons map[string][]byte
}

func newTray(a *App) *tray {
	t := &tray{app: a, icons: map[string][]byte{}}
	for tone, c := range toneColours {
		t.icons[tone] = tint(trayGlyph, c)
	}
	t.menu = a.wails.NewMenu()
	t.menu.Add("Abrir o " + platform.AppName).OnClick(func(*application.Context) { a.showWindow("") })
	t.status = t.menu.Add("Iniciando…").SetEnabled(false)
	t.update = t.menu.Add("").SetHidden(true).OnClick(func(*application.Context) { a.showWindow("/configuracoes#atualizacoes") })
	t.menu.AddSeparator()
	t.autostart = t.menu.AddCheckbox("Iniciar com o sistema", a.Settings().Autostart).OnClick(func(ctx *application.Context) {
		on := ctx.ClickedMenuItem().Checked()
		if err := a.SetAutostart(on); err != nil {
			a.logger.Warn("autostart could not be changed", "error", err)
			ctx.ClickedMenuItem().SetChecked(!on)
		}
	})
	t.menu.AddSeparator()
	t.menu.Add("Sair do " + platform.AppName).OnClick(func(*application.Context) { a.quit() })

	t.icon = a.wails.SystemTray.New()
	t.icon.SetIcon(t.icons["warn"])
	t.icon.SetTooltip(platform.AppName)
	t.icon.SetMenu(t.menu)
	if runtime.GOOS != "darwin" {
		// Windows and Linux open the window on a click and keep the menu for
		// the right button; the macOS menu bar opens the menu either way.
		t.icon.OnClick(func() { a.showWindow("") })
		t.icon.OnRightClick(func() { t.icon.OpenMenu() })
	}
	return t
}

// render shows a status: the icon's colour, and the line in the menu.
func (t *tray) render(st daemon.Status) {
	label := st.Title
	if st.Detail != "" {
		label += " · " + st.Detail
	}
	t.mu.Lock()
	changed := st.Tone != t.tone
	t.tone = st.Tone
	t.mu.Unlock()
	if icon, ok := t.icons[st.Tone]; ok && changed {
		t.icon.SetIcon(icon)
	}
	t.icon.SetTooltip(platform.AppName + ": " + label)
	t.status.SetLabel(label)
	t.menu.Update()
}

// offerUpdate adds "install version X" to the menu.
func (t *tray) offerUpdate(version string) {
	if t == nil {
		return
	}
	t.update.SetLabel("Instalar a versão " + version + "…").SetHidden(false)
	t.menu.Update()
}

// setAutostart keeps the menu's checkbox in step with the panel's.
func (t *tray) setAutostart(on bool) {
	if t == nil {
		return
	}
	t.autostart.SetChecked(on)
	t.menu.Update()
}

// tint paints the glyph's shape in one colour.
func tint(glyph []byte, c color.NRGBA) []byte {
	src, err := png.Decode(bytes.NewReader(glyph))
	if err != nil {
		return glyph
	}
	b := src.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, image.Transparent, image.Point{}, draw.Src)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, alpha := src.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(alpha >> 8)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return glyph
	}
	return buf.Bytes()
}
