package panel

import (
	"runtime"
	"strings"
	"testing"
)

// In the desktop app on Linux the window is WebKitGTK, which crashes the app
// on a navigation while a view transition is on: the stylesheet must not turn
// it on there, and the language change fades the new page in instead. A
// browser, the server version's, keeps the cross-fade.
func TestPageTransitionNotInTheLinuxApp(t *testing.T) {
	const crossFade = "@view-transition{navigation:auto}"
	got := pageTransition("linux", true)
	if strings.Contains(got, "view-transition") {
		t.Fatalf("pageTransition(linux, app) = %q, want no view transition", got)
	}
	if !strings.Contains(got, "@media (prefers-reduced-motion:no-preference){[data-language-fade] body{animation:language-fade") {
		t.Errorf("pageTransition(linux, app) = %q, want the fade-in, kept from reduced motion", got)
	}
	for _, c := range []struct {
		goos string
		app  bool
	}{{"linux", false}, {"darwin", true}, {"windows", true}} {
		if got := pageTransition(c.goos, c.app); !strings.Contains(got, crossFade) {
			t.Errorf("pageTransition(%s, app=%v) = %q, want the cross-fade", c.goos, c.app, got)
		}
	}
	// The rule lives only in pageTransition, and every page gets it from there.
	if strings.Contains(stylesheet, "view-transition") {
		t.Error("the base stylesheet turns the view transition on by itself")
	}
	for _, app := range []bool{true, false} {
		if !strings.HasSuffix(pageStylesheet(app), pageTransition(runtime.GOOS, app)) {
			t.Errorf("pageStylesheet(%v) does not carry pageTransition", app)
		}
	}
	theme, err := assets.ReadFile("assets/theme.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(theme), `root.setAttribute("data-language-fade", "")`) {
		t.Error("theme.js does not mark the page that comes back from a language change")
	}
}
