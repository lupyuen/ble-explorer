/*
To build and run:

# Ignore the message "go get: no install location"
go get ./...
go build main.go

# Scan for 5 seconds, connect timeout is 5 seconds
sudo ./main -sd 5s -cd 5s

Based on
https://github.com/JuulLabs-OSS/ble/tree/master/examples/basic/scanner
https://github.com/JuulLabs-OSS/ble/tree/master/examples/basic/explorer
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/JuulLabs-OSS/ble"
	"github.com/JuulLabs-OSS/ble/examples/lib/dev"
	"github.com/pkg/errors"
)

// Command-line options
var (
	device = flag.String("device", "default", "implementation of ble")
	sub    = flag.Duration("sub", 0, "subscribe to notification and indication for a specified period")
	sd     = flag.Duration("sd", 5*time.Second, "scanning duration, 0 for indefinitely")
	cd     = flag.Duration("cd", 1*time.Second, "connect duration, 0 for indefinitely")
	dup    = flag.Bool("dup", false, "allow duplicate reported")
)

// List of devices scanned
var devices []ble.Advertisement

func main() {
	// Parse the command-line options
	flag.Parse()

	// Open the BLE interface
	d, err := dev.NewDevice(*device)
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)

	// Scan for specified duration, or until interrupted by user
	fmt.Printf("Scanning for %s...\n", *sd)
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *sd))
	chkErr(ble.Scan(ctx, *dup, advHandler, nil))

	// Connect to every device scanned and display services
	fmt.Printf("Connecting to devices...\n")
	previousDevices := make(map[string]bool)
	for _, device := range devices {
		addr := device.Addr().String()
		if previousDevices[addr] {
			continue // Skip if we have connected to this device
		}
		previousDevices[addr] = true
		// Connect to the device and display services
		connect(device)
	}
	fmt.Printf("Done\n")
}

// Handle each device scanned
func advHandler(a ble.Advertisement) {
	if !a.Connectable() {
		return // Skip non-connectable devices
	}
	// Append the device for connecting later
	devices = append(devices, a)
}

// Connect to the device and explore services
func connect(device ble.Advertisement) {
	// Search for addr
	filter := func(a ble.Advertisement) bool {
		return a.Addr().String() == device.Addr().String()
	}

	// Scan for specified durantion, or until interrupted by user.
	// fmt.Printf("Connecting for %s...\n", *sd)
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *cd))
	cln, err := ble.Connect(ctx, filter)
	if err != nil {
		// fmt.Printf("can't connect : %s\n", err)
		return
	}

	// Make sure we had the chance to print out the message.
	done := make(chan struct{})
	// Normally, the connection is disconnected by us after our exploration.
	// However, it can be asynchronously disconnected by the remote peripheral.
	// So we wait(detect) the disconnection in the go routine.
	go func() {
		<-cln.Disconnected()
		// fmt.Printf("[ %s ] is disconnected \n", cln.Addr())
		close(done)
	}()

	// fmt.Printf("Discovering profile...\n")
	p, err := cln.DiscoverProfile(true)
	if err != nil {
		// fmt.Printf("can't discover profile: %s\n", err)
	} else {
		// Start the exploration
		explore(cln, p, device)
	}

	// Disconnect the connection. (On OS X, this might take a while.)
	// fmt.Printf("Disconnecting [ %s ]... (this might take up to few seconds on OS X)\n", cln.Addr())
	cln.CancelConnection()

	<-done
}

// ServiceID is the GATT Service ID to be searched, in lowercase
const ServiceID = "b82ab3fc15954f6a80f0fe094cc218f9"

// Explore the services for the device
func explore(cln ble.Client, p *ble.Profile, a ble.Advertisement) error {
	// Find the GATT service
	foundService := false
	for _, s := range p.Services {
		if s.UUID.String() == ServiceID {
			foundService = true
			break
		}
	}
	if !foundService {
		// Quit if service not found
		return nil
	}
	// Display the device
	showDevice(a)
	// For all GATT services...
	for _, s := range p.Services {
		if s.UUID.String() != ServiceID {
			// Show only our service
			continue
		}
		fmt.Printf("    Service: %s %s, Handle (0x%02X)\n", s.UUID, ble.Name(s.UUID), s.Handle)

		for _, c := range s.Characteristics {
			fmt.Printf("      Characteristic: %s %s, Property: 0x%02X (%s), Handle(0x%02X), VHandle(0x%02X)\n",
				c.UUID, ble.Name(c.UUID), c.Property, propString(c.Property), c.Handle, c.ValueHandle)
			if (c.Property & ble.CharRead) != 0 {
				b, err := cln.ReadLongCharacteristic(c)
				if err != nil {
					// fmt.Printf("Failed to read characteristic: %s\n", err)
					continue
				}
				fmt.Printf("        Value         %x | %q\n", b, b)
			}

			for _, d := range c.Descriptors {
				fmt.Printf("        Descriptor: %s %s, Handle(0x%02x)\n", d.UUID, ble.Name(d.UUID), d.Handle)
				b, err := cln.ReadDescriptor(d)
				if err != nil {
					// fmt.Printf("Failed to read descriptor: %s\n", err)
					continue
				}
				fmt.Printf("        Value         %x | %q\n", b, b)
			}

			if *sub != 0 {
				// Don't bother to subscribe the Service Changed characteristics.
				if c.UUID.Equal(ble.ServiceChangedUUID) {
					continue
				}

				// Don't touch the Apple-specific Service/Characteristic.
				// Service: D0611E78BBB44591A5F8487910AE4366
				// Characteristic: 8667556C9A374C9184ED54EE27D90049, Property: 0x18 (WN),
				//   Descriptor: 2902, Client Characteristic Configuration
				//   Value         0000 | "\x00\x00"
				if c.UUID.Equal(ble.MustParse("8667556C9A374C9184ED54EE27D90049")) {
					continue
				}

				if (c.Property & ble.CharNotify) != 0 {
					fmt.Printf("\n-- Subscribe to notification for %s --\n", *sub)
					h := func(req []byte) { fmt.Printf("Notified: %q [ % X ]\n", string(req), req) }
					if err := cln.Subscribe(c, false, h); err != nil {
						log.Fatalf("subscribe failed: %s", err)
					}
					time.Sleep(*sub)
					if err := cln.Unsubscribe(c, false); err != nil {
						log.Fatalf("unsubscribe failed: %s", err)
					}
					fmt.Printf("-- Unsubscribe to notification --\n")
				}
				if (c.Property & ble.CharIndicate) != 0 {
					fmt.Printf("\n-- Subscribe to indication of %s --\n", *sub)
					h := func(req []byte) { fmt.Printf("Indicated: %q [ % X ]\n", string(req), req) }
					if err := cln.Subscribe(c, true, h); err != nil {
						log.Fatalf("subscribe failed: %s", err)
					}
					time.Sleep(*sub)
					if err := cln.Unsubscribe(c, true); err != nil {
						log.Fatalf("unsubscribe failed: %s", err)
					}
					fmt.Printf("-- Unsubscribe to indication --\n")
				}
			}
		}
		fmt.Printf("\n")
	}
	return nil
}

// Display the scanned device
func showDevice(a ble.Advertisement) {
	if a.Connectable() {
		fmt.Printf("[%s] RSSI %3d:\n", a.Addr(), a.RSSI())
	} else {
		// fmt.Printf("[%s] N %3d:", a.Addr(), a.RSSI())
		return // Skip non-connectable devices
	}

	comma := ""
	if len(a.LocalName()) > 0 {
		fmt.Printf("Name: %s\n", a.LocalName())
		comma = ""
	}
	if len(a.Services()) > 0 {
		fmt.Printf("%sSvcs: %v\n", comma, a.Services())
		comma = ""
	}
	if len(a.ManufacturerData()) > 0 {
		fmt.Printf("%sManu: %X", comma, a.ManufacturerData())
	}
	fmt.Printf("\n")
}

func propString(p ble.Property) string {
	var s string
	for k, v := range map[ble.Property]string{
		ble.CharBroadcast:   "B",
		ble.CharRead:        "R",
		ble.CharWriteNR:     "w",
		ble.CharWrite:       "W",
		ble.CharNotify:      "N",
		ble.CharIndicate:    "I",
		ble.CharSignedWrite: "S",
		ble.CharExtended:    "E",
	} {
		if p&k != 0 {
			s += v
		}
	}
	return s
}

func chkErr(err error) {
	switch errors.Cause(err) {
	case nil:
	case context.DeadlineExceeded:
		fmt.Printf("Scanned %d devices\n", len(devices))
	case context.Canceled:
		fmt.Printf("canceled\n")
	default:
		log.Fatalf(err.Error())
	}
}
