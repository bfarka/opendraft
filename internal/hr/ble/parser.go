package ble

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrInvalidPacket indicates a malformed GATT Heart Rate Measurement payload.
var ErrInvalidPacket = errors.New("invalid heart rate measurement packet")

// ParseHeartRateMeasurement decodes a Bluetooth SIG GATT 0x2A37 Heart Rate Measurement payload
// and returns the heart rate in beats per minute (BPM).
func ParseHeartRateMeasurement(data []byte) (uint8, error) {
	if len(data) < 2 {
		return 0, fmt.Errorf("%w: length %d < 2", ErrInvalidPacket, len(data))
	}

	flags := data[0]
	is16Bit := flags&0x01 != 0

	if !is16Bit {
		return data[1], nil
	}

	if len(data) < 3 {
		return 0, fmt.Errorf("%w: 16-bit flag set but length %d < 3", ErrInvalidPacket, len(data))
	}

	val := binary.LittleEndian.Uint16(data[1:3])
	if val > 255 {
		return 255, nil
	}
	return uint8(val), nil
}
