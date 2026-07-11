//go:build !linux

package ui

import "github.com/getlantern/systray"

func setTrayTitle(title string) {
	systray.SetTitle(title)
}
