package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/device"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/ui"
)

var (
	hidManager  *device.HIDRawManager
	trayManager *ui.TrayManager
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Starting goarctis...")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received interrupt signal, cleaning up...")
		cleanup()
		os.Exit(0)
	}()

	systray.Run(onReady, onExit)
}

func onReady() {
	// Initialize UI
	trayManager = ui.NewTrayManager()
	trayManager.Initialize()

	// Initialize HID manager
	hidManager = device.NewHIDRawManager()
	hidManager.SetOnStateChange(onStateChange)

	// Try to find and connect
	go func() {
		if err := hidManager.FindDevices(); err != nil {
			log.Printf("Failed to find devices: %v", err)
			trayManager.SetStatus("Device not found")
			return
		}

		trayManager.SetStatus("🎧 Connected")

		// Start listening
		go hidManager.Listen()
	}()

	// Handle quit button
	go func() {
		<-trayManager.QuitChannel()
		log.Println("Quit clicked")
		systray.Quit()
	}()
}

func onStateChange(state protocol.DeviceState) {
	trayManager.UpdateState(state)
}

func cleanup() {
	log.Println("Cleaning up...")

	if hidManager != nil {
		log.Println("Closing HID devices")
		hidManager.Close()
	}

	log.Println("Cleanup complete")
}

func onExit() {
	log.Println("onExit called")
	cleanup()
}
