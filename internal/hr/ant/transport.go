package ant

import (
	"context"
	"errors"
	"io"
)

// ErrNoStick indicates that no supported ANT+ USB stick was found (or USB is unsupported on this OS).
var ErrNoStick = errors.New("ant: no ANT+ USB stick found")

// Transport abstracts raw packet/stream I/O to an ANT+ USB stick.
type Transport interface {
	io.ReadWriteCloser
}

// Opener opens a Transport to an available ANT+ USB stick.
type Opener func(ctx context.Context) (Transport, error)
