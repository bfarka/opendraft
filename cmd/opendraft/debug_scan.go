package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"tinygo.org/x/bluetooth"

	"github.com/bfarka/opendraft/internal/hr/ble"
)

func runDebugScan() error {
	fmt.Println("Running 5s diagnostic scan (BLE + mDNS Shelly)...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// 1. Raw BLE scan
	wg.Add(1)
	go func() {
		defer wg.Done()
		adapter := bluetooth.DefaultAdapter
		if err := adapter.Enable(); err != nil {
			fmt.Printf("[BLE] Enable error: %v\n", err)
			return
		}
		fmt.Println("[BLE] Adapter enabled, scanning all devices...")
		seen := make(map[string]struct{})
		var mu sync.Mutex
		go func() {
			<-ctx.Done()
			_ = adapter.StopScan()
		}()
		err := adapter.Scan(func(_ *bluetooth.Adapter, res bluetooth.ScanResult) {
			id := res.Address.String()
			mu.Lock()
			_, ok := seen[id]
			seen[id] = struct{}{}
			mu.Unlock()
			if ok {
				return
			}
			hasHR := res.HasServiceUUID(bluetooth.New16BitUUID(0x180D))
			fmt.Printf("[BLE] addr=%s rssi=%d name=%q has0x180D=%v\n", id, res.RSSI, res.LocalName(), hasHR)
		})
		if err != nil {
			fmt.Printf("[BLE] Scan error: %v\n", err)
		}
	}()

	// 2. mDNS scan for Shelly (_http._tcp and _shelly._tcp)
	for _, svc := range []string{"_http._tcp", "_shelly._tcp"} {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			resolver, err := zeroconf.NewResolver(nil)
			if err != nil {
				fmt.Printf("[mDNS %s] NewResolver error: %v\n", service, err)
				return
			}
			entries := make(chan *zeroconf.ServiceEntry, 32)
			go func() {
				for e := range entries {
					fmt.Printf("[mDNS %s] instance=%q host=%q ip4=%v port=%d txt=%v\n",
						service, e.Instance, e.HostName, e.AddrIPv4, e.Port, e.Text)
				}
			}()
			fmt.Printf("[mDNS] Browsing %s local. ...\n", service)
			if err := resolver.Browse(ctx, service, "local.", entries); err != nil {
				fmt.Printf("[mDNS %s] Browse error: %v\n", service, err)
			}
		}(svc)
	}

	wg.Wait()
	fmt.Println("Diagnostic scan complete.")
	return nil
}

func runDebugHR() error {
	src := ble.NewSource()
	defer src.Close()

	scanCtx, scanCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer scanCancel()

	fmt.Println("[debug-hr] Scanning for BLE HR monitor...")
	ch, err := src.Scan(scanCtx)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	dev, ok := <-ch
	if !ok {
		return fmt.Errorf("no BLE HR device found")
	}
	scanCancel()
	fmt.Printf("[debug-hr] Found %q (%s), connecting...\n", dev.Name, dev.ID)

	connCtx, connCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connCancel()
	hrCh, err := src.Connect(connCtx, dev.ID)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	fmt.Println("[debug-hr] Connected! Waiting for BPM readings...")
	for r := range hrCh {
		fmt.Printf("[debug-hr] BPM=%d at %s\n", r.BPM, r.Timestamp.Format(time.RFC3339))
	}
	fmt.Println("[debug-hr] Stream ended.")
	return nil
}
