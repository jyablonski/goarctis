package main

import (
	"fmt"
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
	deviceManager *device.DeviceManager
	trayManager   *ui.TrayManager
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

	// Initialize device manager
	deviceManager = device.NewDeviceManager()
	deviceManager.SetOnStateChange(onStateChange)

	// Discover and start devices
	go func() {
		if err := deviceManager.DiscoverDevices(); err != nil {
			log.Printf("Failed to discover devices: %v", err)
			trayManager.SetStatus("No devices found")
			return
		}

		devices := deviceManager.GetAllDevices()
		if len(devices) == 0 {
			trayManager.SetStatus("No devices found")
			return
		}

		// Build status message
		deviceNames := make([]string, 0, len(devices))
		for _, dev := range devices {
			deviceNames = append(deviceNames, dev.GetName())
		}
		trayManager.SetStatus(fmt.Sprintf("Connected: %d device(s)", len(devices)))

		// Start monitoring all devices
		if err := deviceManager.StartAll(); err != nil {
			log.Printf("Failed to start some devices: %v", err)
		}
	}()

	// Handle quit button
	go func() {
		<-trayManager.QuitChannel()
		log.Println("Quit clicked")
		systray.Quit()
	}()
}

func onStateChange(deviceID string, state protocol.DeviceState) {
	trayManager.UpdateDeviceState(deviceID, state)
}

func cleanup() {
	log.Println("Cleaning up...")

	if deviceManager != nil {
		log.Println("Closing all devices")
		deviceManager.CloseAll()
	}

	log.Println("Cleanup complete")
}

func onExit() {
	log.Println("onExit called")
	cleanup()
}
