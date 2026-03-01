package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/device"
	"github.com/jyablonski/goarctis/pkg/docker"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/ui"
	"github.com/jyablonski/goarctis/pkg/version"
)

var (
	deviceManager   *device.DeviceManager
	trayManager     *ui.TrayManager
	dockerMonitor   *docker.Monitor
	disableGameBuds bool
	disableRazer    bool
)

func main() {
	// Parse command line flags
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.BoolVar(&disableGameBuds, "disable-gamebuds", false, "Disable SteelSeries GameBuds monitoring")
	flag.BoolVar(&disableRazer, "disable-razer", false, "Disable Razer device monitoring")
	flag.Parse()

	if *showVersion {
		fmt.Printf("goarctis version %s\n", version.Version)
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Starting goarctis version %s...", version.Version)

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
	trayManager.Initialize(ui.TrayConfig{
		DisableGameBuds: disableGameBuds,
		DisableRazer:    disableRazer,
	})

	// Initialize device manager
	deviceManager = device.NewDeviceManager()
	deviceManager.SetOnStateChange(onStateChange)

	// Discover and start devices
	go func() {
		if err := deviceManager.DiscoverDevices(device.DiscoveryConfig{
			DisableGameBuds: disableGameBuds,
			DisableRazer:    disableRazer,
		}); err != nil {
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

	// Initialize Docker monitor (poll every 10 seconds)
	dockerMonitor = docker.NewMonitor(10 * time.Second)
	dockerMonitor.SetOnChange(func(state docker.DockerState) {
		trayManager.UpdateDockerState(state)
	})
	dockerMonitor.Start()

	// Handle "Stop All Containers" button
	go func() {
		runner := &docker.ExecCommandRunner{}
		for range trayManager.DockerStopAllChannel() {
			log.Println("Stopping all Docker containers...")
			stopped, err := docker.StopAllContainers(runner)
			if err != nil {
				log.Printf("Error stopping containers: %v", err)
			} else {
				log.Printf("Stopped %d container(s)", stopped)
			}
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

	if dockerMonitor != nil {
		log.Println("Stopping Docker monitor")
		dockerMonitor.Stop()
	}

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
