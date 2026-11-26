//go:build !linux

package ant

import "context"

// OpenUSBStick returns ErrNoStick on non-Linux platforms where Linux usbfs is unavailable.
func OpenUSBStick(_ context.Context) (Transport, error) {
	return nil, ErrNoStick
}
