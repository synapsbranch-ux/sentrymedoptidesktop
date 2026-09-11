//go:build !linux

package server

import "context"

// Only BlueZ exposes a raw RFCOMM socket. Everywhere else the operating system
// pairs the printer and publishes it as a serial port, which the device-path
// transport opens directly.
func writeBluetoothRFCOMM(context.Context, string, int, []byte) error {
	return printerFailure("PRINTER_CONNECTION_FAILED", "Select the Bluetooth serial port provided by this operating system.", nil)
}
