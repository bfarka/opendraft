package ant

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	syncByte byte = 0xA4

	// ANT+ message IDs
	msgChannelEventOrResponse byte = 0x40
	msgUnassignChannel        byte = 0x41
	msgAssignChannel          byte = 0x42
	msgSetChannelPeriod       byte = 0x43
	msgSetChannelTimeout      byte = 0x44
	msgSetChannelRFFreq       byte = 0x45
	msgSetNetworkKey          byte = 0x46
	msgResetSystem            byte = 0x4A
	msgOpenChannel            byte = 0x4B
	msgCloseChannel           byte = 0x4C
	msgBroadcastData          byte = 0x4E
	msgAcknowledgedData       byte = 0x4F
	msgSetChannelID           byte = 0x51
	msgOpenRxScanMode         byte = 0x5B
	msgLibConfig              byte = 0x6E
	msgStartupMessage         byte = 0x6F

	// ANT+ Heart Rate Monitor (HRM) profile constants
	hrmDeviceType    byte   = 120  // 0x78
	hrmChannelPeriod uint16 = 8070 // 4.06 Hz
	antPlusRFFreq    byte   = 57   // 2457 MHz
	libConfigAll     byte   = 0xE0 // Channel ID (0x80) | RSSI (0x40) | Timestamp (0x20)
	extFlagChannelID byte   = 0x80
	extFlagRSSI      byte   = 0x40
	extFlagTimestamp byte   = 0x20
)

// antPlusNetworkKey is the public ANT+ network key.
var antPlusNetworkKey = [8]byte{0xB9, 0xA5, 0x21, 0xFB, 0xBD, 0x72, 0xC3, 0x45}

// ErrChecksumMismatch indicates a corrupted ANT+ frame.
var ErrChecksumMismatch = errors.New("ant: checksum mismatch")

// Frame represents a decoded ANT+ serial/USB message.
type Frame struct {
	MsgID   byte
	Payload []byte
}

// EncodeFrame serializes an ANT+ message with sync byte, length, ID, payload, and XOR checksum.
func EncodeFrame(msgID byte, payload ...byte) []byte {
	frame := make([]byte, 0, len(payload)+4)
	frame = append(frame, syncByte, byte(len(payload)), msgID)
	frame = append(frame, payload...)
	var cs byte
	for _, b := range frame {
		cs ^= b
	}
	frame = append(frame, cs)
	return frame
}

func cmdResetSystem() []byte {
	return EncodeFrame(msgResetSystem, 0x00)
}

func cmdSetNetworkKey(network byte) []byte {
	payload := make([]byte, 9)
	payload[0] = network
	copy(payload[1:], antPlusNetworkKey[:])
	return EncodeFrame(msgSetNetworkKey, payload...)
}

func cmdAssignChannel(channel, channelType, network byte) []byte {
	return EncodeFrame(msgAssignChannel, channel, channelType, network)
}

func cmdSetChannelID(channel byte, deviceNum uint16, deviceType, transType byte) []byte {
	var lo, hi byte
	lo = byte(deviceNum)
	hi = byte(deviceNum >> 8)
	return EncodeFrame(msgSetChannelID, channel, lo, hi, deviceType, transType)
}

func cmdSetChannelPeriod(channel byte, period uint16) []byte {
	return EncodeFrame(msgSetChannelPeriod, channel, byte(period), byte(period>>8))
}

func cmdSetChannelRFFreq(channel, freq byte) []byte {
	return EncodeFrame(msgSetChannelRFFreq, channel, freq)
}

func cmdSetChannelTimeout(channel, timeout byte) []byte {
	return EncodeFrame(msgSetChannelTimeout, channel, timeout)
}

func cmdLibConfig(flags byte) []byte {
	return EncodeFrame(msgLibConfig, 0x00, flags)
}

func cmdOpenChannel(channel byte) []byte {
	return EncodeFrame(msgOpenChannel, channel)
}

func cmdCloseChannel(channel byte) []byte {
	return EncodeFrame(msgCloseChannel, channel)
}

func cmdOpenRxScanMode() []byte {
	return EncodeFrame(msgOpenRxScanMode, 0x00, 0x01)
}

// FrameDecoder incrementally parses ANT+ frames from a raw byte stream.
type FrameDecoder struct {
	buf []byte
}

// Feed appends raw bytes and returns all complete valid frames decoded so far.
// Corrupted or out-of-sync bytes are skipped.
func (d *FrameDecoder) Feed(data []byte) []Frame {
	d.buf = append(d.buf, data...)
	var frames []Frame

	for {
		syncIdx := -1
		for i, b := range d.buf {
			if b == syncByte {
				syncIdx = i
				break
			}
		}
		if syncIdx < 0 {
			d.buf = d.buf[:0]
			break
		}
		if syncIdx > 0 {
			d.buf = d.buf[syncIdx:]
		}
		if len(d.buf) < 4 {
			break
		}
		length := int(d.buf[1])
		totalLen := length + 4
		if len(d.buf) < totalLen {
			break
		}

		var cs byte
		for i := 0; i < totalLen-1; i++ {
			cs ^= d.buf[i]
		}
		if cs != d.buf[totalLen-1] {
			// Bad checksum: advance past this sync byte and resynchronize.
			d.buf = d.buf[1:]
			continue
		}

		payload := make([]byte, length)
		copy(payload, d.buf[3:3+length])
		frames = append(frames, Frame{
			MsgID:   d.buf[2],
			Payload: payload,
		})
		d.buf = d.buf[totalLen:]
	}

	return frames
}

// HRBroadcast holds a parsed ANT+ HRM broadcast packet.
type HRBroadcast struct {
	Channel      byte
	BPM          uint8
	HasExtID     bool
	DeviceNumber uint16
	DeviceType   byte
	TransType    byte
	HasRSSI      bool
	RSSI         int16
}

// ParseHRBroadcast parses an ANT+ Broadcast Data (0x4E) or Acknowledged Data (0x4F) payload.
func ParseHRBroadcast(payload []byte) (HRBroadcast, error) {
	if len(payload) < 9 {
		return HRBroadcast{}, fmt.Errorf("ant: broadcast payload too short (%d bytes)", len(payload))
	}
	res := HRBroadcast{
		Channel: payload[0],
		BPM:     payload[8],
	}
	if len(payload) == 9 {
		return res, nil
	}

	flag := payload[9]
	idx := 10

	if flag&extFlagChannelID != 0 {
		if len(payload) < idx+4 {
			return res, nil
		}
		res.HasExtID = true
		res.DeviceNumber = binary.LittleEndian.Uint16(payload[idx : idx+2])
		res.DeviceType = payload[idx+2]
		res.TransType = payload[idx+3]
		idx += 4
	}

	if flag&extFlagRSSI != 0 {
		if len(payload) < idx+3 {
			return res, nil
		}
		res.HasRSSI = true
		res.RSSI = int16(int8(payload[idx+1]))
		idx += 3
	}

	if flag&extFlagTimestamp != 0 {
		if len(payload) >= idx+2 {
			idx += 2
		}
	}

	return res, nil
}
