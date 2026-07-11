package ui

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
#include <stdlib.h>

void schedule_tray_title(char *title);
*/
import "C"

import (
	"unsafe"

	"github.com/getlantern/systray"
)

func setTrayTitle(title string) {
	C.schedule_tray_title(C.CString(title))
}

//export goSetTrayTitle
func goSetTrayTitle(data unsafe.Pointer) C.gboolean {
	title := (*C.char)(data)
	systray.SetTitle(C.GoString(title))
	C.free(data)
	return C.gboolean(0)
}
