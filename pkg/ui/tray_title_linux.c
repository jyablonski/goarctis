//go:build linux

#include <glib.h>

extern gboolean goSetTrayTitle(gpointer data);

void schedule_tray_title(char *title) {
	// getlantern/systray calls AppIndicator's title and label setters on the
	// calling goroutine. Tray renders run in a background goroutine, but GTK UI
	// updates must run on its main loop. Queue the Go callback there so its
	// eventual systray.SetTitle call is made from the GTK thread.
	g_idle_add(goSetTrayTitle, title);
}
