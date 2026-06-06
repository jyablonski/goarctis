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
	"github.com/jyablonski/goarctis/pkg/selfupdate"
	"github.com/jyablonski/goarctis/pkg/system"
	"github.com/jyablonski/goarctis/pkg/ui"
	"github.com/jyablonski/goarctis/pkg/version"
)

var (
	deviceManager   *device.DeviceManager
	trayManager     *ui.TrayManager
	dockerMonitor   *docker.Monitor
	systemMonitor   *system.Monitor
	disableGameBuds bool
	disableRazer    bool
	disableHyperX   bool
	disableSystem   bool
	gpuGuardConfig  system.GovernorConfig
)

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	doSelfUpdate := flag.Bool("self-update", false, "Update goarctis to the latest release and restart the service")
	flag.BoolVar(&disableGameBuds, "disable-gamebuds", false, "Disable SteelSeries GameBuds monitoring")
	flag.BoolVar(&disableRazer, "disable-razer", false, "Disable Razer device monitoring")
	flag.BoolVar(&disableHyperX, "disable-hyperx", false, "Disable HyperX Cloud Alpha Wireless monitoring")
	flag.BoolVar(&disableSystem, "disable-system", false, "Disable CPU and memory monitoring")
	gpuThermalGuard := flag.Bool("gpu-thermal-guard", false, "Auto-reduce NVIDIA GPU power limit above a temperature threshold (needs root)")
	flag.Parse()

	gpuGuardConfig = system.DefaultGovernorConfig(*gpuThermalGuard)

	if *showVersion {
		fmt.Printf("goarctis version %s\n", version.Version)
		os.Exit(0)
	}

	if *doSelfUpdate {
		if err := selfupdate.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "self-update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Starting goarctis version %s...", version.Version)

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
	trayManager = ui.NewTrayManager()
	trayManager.Initialize(ui.TrayConfig{
		DisableGameBuds: disableGameBuds,
		DisableRazer:    disableRazer,
		DisableHyperX:   disableHyperX,
		DisableSystem:   disableSystem,
	})

	deviceManager = device.NewDeviceManager()
	deviceManager.SetOnStateChange(onStateChange)

	go func() {
		if err := deviceManager.DiscoverDevices(device.DiscoveryConfig{
			DisableGameBuds: disableGameBuds,
			DisableRazer:    disableRazer,
			DisableHyperX:   disableHyperX,
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

		trayManager.SetStatus(fmt.Sprintf("Connected: %d device(s)", len(devices)))

		if err := deviceManager.StartAll(); err != nil {
			log.Printf("Failed to start some devices: %v", err)
		}
	}()

	dockerMonitor = docker.NewMonitor(10 * time.Second)
	dockerMonitor.SetOnChange(func(state docker.DockerState) {
		trayManager.UpdateDockerState(state)
	})
	dockerMonitor.Start()

	if !disableSystem {
		systemMonitor = system.NewMonitorWithConfig(system.DefaultPollInterval, gpuGuardConfig)
		systemMonitor.SetOnChange(func(state system.State) {
			trayManager.UpdateSystemState(state)
		})
		systemMonitor.Start()
	}

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

	if systemMonitor != nil {
		log.Println("Stopping system monitor")
		systemMonitor.Stop()
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
