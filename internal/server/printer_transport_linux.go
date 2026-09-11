//go:build linux

package server

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

func writeBluetoothRFCOMM(ctx context.Context, address string, channel int, payload []byte) error {
	parts := strings.Split(address, ":")
	if len(parts) != 6 || channel < 1 || channel > 30 {
		return errors.New("invalid Bluetooth RFCOMM destination")
	}
	var target [6]uint8
	for index, part := range parts {
		value, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return errors.New("invalid Bluetooth address")
		}
		target[5-index] = uint8(value)
	}
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM, unix.BTPROTO_RFCOMM)
	if err != nil {
		return errors.New("Bluetooth RFCOMM is unavailable")
	}
	var once sync.Once
	closeSocket := func() { once.Do(func() { _ = unix.Close(fd) }) }
	defer closeSocket()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			closeSocket()
		case <-done:
		}
	}()
	defer close(done)
	if err = unix.Connect(fd, &unix.SockaddrRFCOMM{Addr: target, Channel: uint8(channel)}); err != nil {
		return errors.New("printer is offline, out of range, or Bluetooth permission was denied")
	}
	for len(payload) > 0 {
		written, writeErr := unix.Write(fd, payload)
		if writeErr != nil {
			return errors.New("printer write failed")
		}
		if written == 0 {
			return errors.New("printer connection closed")
		}
		payload = payload[written:]
	}
	return nil
}
