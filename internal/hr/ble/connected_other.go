//go:build !darwin

package ble

import (
	"context"

	"github.com/bfarka/opendraft/internal/domain"
)

// ConnectedHRDevices is a no-op on non-darwin platforms where shared
// CoreBluetooth connected-peripheral retrieval is not applicable.
func ConnectedHRDevices(_ context.Context) []domain.HRDevice {
	return nil
}
