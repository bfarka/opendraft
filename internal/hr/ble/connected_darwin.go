//go:build darwin

package ble

import (
	"context"
	"sync"
	"time"

	"github.com/tinygo-org/cbgo"

	"github.com/bfarka/opendraft/internal/domain"
)

type connectedDelegate struct {
	cbgo.CentralManagerDelegateBase
	ch   chan struct{}
	once sync.Once
}

func (d *connectedDelegate) CentralManagerDidUpdateState(cm cbgo.CentralManager) {
	if cm.State() == cbgo.ManagerStatePoweredOn {
		d.once.Do(func() { close(d.ch) })
	}
}

var (
	cbOnce    sync.Once
	cbManager cbgo.CentralManager
	cbReady   chan struct{}
)

func initConnectedManager() {
	cbOnce.Do(func() {
		cbReady = make(chan struct{})
		dlg := &connectedDelegate{ch: cbReady}
		cm := cbgo.NewCentralManager(&cbgo.ManagerOpts{ShowPowerAlert: false})
		cm.SetDelegate(dlg)
		if cm.State() == cbgo.ManagerStatePoweredOn {
			dlg.once.Do(func() { close(cbReady) })
		}
		cbManager = cm
	})
}

// ConnectedHRDevices returns BLE peripherals already connected at the macOS
// CoreBluetooth layer that expose the Heart Rate Service (0x180D), such as
// straps currently connected to another app like Rouvy.
func ConnectedHRDevices(ctx context.Context) []domain.HRDevice {
	initConnectedManager()

	if cbManager.State() != cbgo.ManagerStatePoweredOn {
		select {
		case <-cbReady:
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
			return nil
		}
	}

	hrUUID, err := cbgo.ParseUUID("180D")
	if err != nil {
		return nil
	}

	prphs := cbManager.RetrieveConnectedPeripheralsWithServices([]cbgo.UUID{hrUUID})
	devices := make([]domain.HRDevice, 0, len(prphs))
	for _, p := range prphs {
		id := p.Identifier().String()
		if id == "" {
			continue
		}
		name := p.Name()
		if name == "" {
			name = id
		}
		devices = append(devices, domain.HRDevice{
			ID:   id,
			Name: name,
			RSSI: 0,
		})
	}
	return devices
}
