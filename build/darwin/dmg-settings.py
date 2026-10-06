# dmgbuild settings for WhatsApp-MCP.dmg: the app on the left, a shortcut to
# Applications on the right, over a background that says to drag one onto the
# other. scripts/build-macos.sh runs it from the repository root and passes
# the app's path with -D app=…
import os.path

app = defines["app"]  # noqa: F821 (dmgbuild provides it)

format = "UDZO"
files = [app]
symlinks = {"Applications": "/Applications"}
icon = "build/darwin/icon.icns"

# dmgbuild picks up dmg-background@2x.png beside it for Retina screens.
background = "build/darwin/dmg-background.png"
window_rect = ((200, 120), (640, 480))
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
default_view = "icon-view"
icon_size = 128
text_size = 13
icon_locations = {
    os.path.basename(app): (180, 185),
    "Applications": (460, 185),
}
