//go:build !linux

package server

import (
	"context"
	"errors"
)

func writeBluetoothRFCOMM(context.Context, string, int, []byte) error {
	return errors.New("select the Bluetooth serial port provided by this operating system")
}
