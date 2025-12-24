package main

import (
	"fmt"
	"log"

	"github.com/jyablonski/goarctis/pkg/device"
)

// Simple test script to verify Razer device discovery without running full app
func main() {
	fmt.Println("Testing Razer device discovery...")

	devices, err := device.DiscoverRazerDevices()
	if err != nil {
		log.Printf("Error discovering Razer devices: %v", err)
		fmt.Println("\nPossible reasons:")
		fmt.Println("  - OpenRazer daemon not running")
		fmt.Println("  - No Razer devices with battery support connected")
		fmt.Println("  - D-Bus connection failed")
		return
	}

	if len(devices) == 0 {
		fmt.Println("No Razer devices with battery support found")
		return
	}

	fmt.Printf("\nFound %d Razer device(s):\n\n", len(devices))
	for i, dev := range devices {
		fmt.Printf("Device %d:\n", i+1)
		fmt.Printf("  Name: %s\n", dev.GetName())
		fmt.Printf("  ID: %s\n", dev.GetID())
		fmt.Printf("  Type: %s\n", dev.GetType())

		state := dev.GetState()
		if state.Battery != nil {
			fmt.Printf("  Battery: %d%%\n", *state.Battery)
		} else {
			fmt.Printf("  Battery: Not available\n")
		}

		if state.IsCharging != nil {
			fmt.Printf("  Charging: %v\n", *state.IsCharging)
		}

		fmt.Println()
	}
}
