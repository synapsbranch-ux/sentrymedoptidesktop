//go:build linux

package server

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

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
	err := writeRFCOMMOnce(ctx, target, channel, payload)
	if shouldReconnectBluetooth(err) {
		// BlueZ may know the paired device while its ACL link is asleep. Wake it
		// once, using a validated address as a fixed argv value (never a shell),
		// then open a fresh RFCOMM socket. Printing stays bounded by the request.
		wakeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		_ = exec.CommandContext(wakeCtx, "bluetoothctl", "connect", address).Run()
		cancel()
		if ctx.Err() == nil {
			err = writeRFCOMMOnce(ctx, target, channel, payload)
		}
	}
	return describeRFCOMMError(err)
}

func writeRFCOMMOnce(ctx context.Context, target [6]uint8, channel int, payload []byte) error {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.BTPROTO_RFCOMM)
	if err != nil {
		return fmt.Errorf("create RFCOMM socket: %w", err)
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
	// Match BlueZ's rfcomm client: select a local adapter/channel before the
	// remote connect. Some controllers reject an implicit source selection.
	if err = unix.Bind(fd, &unix.SockaddrRFCOMM{}); err != nil {
		return fmt.Errorf("bind RFCOMM socket: %w", err)
	}
	if err = unix.Connect(fd, &unix.SockaddrRFCOMM{Addr: target, Channel: uint8(channel)}); err != nil {
		return fmt.Errorf("connect RFCOMM socket: %w", err)
	}
	for len(payload) > 0 {
		written, writeErr := unix.Write(fd, payload)
		if writeErr != nil {
			return fmt.Errorf("write RFCOMM socket: %w", writeErr)
		}
		if written == 0 {
			return errors.New("printer connection closed")
		}
		payload = payload[written:]
	}
	return nil
}

func shouldReconnectBluetooth(err error) bool {
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.EHOSTDOWN) || errors.Is(err, unix.EHOSTUNREACH) ||
		errors.Is(err, unix.ENETDOWN) || errors.Is(err, unix.ENETUNREACH) || errors.Is(err, unix.ETIMEDOUT)
}

func describeRFCOMMError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("printer connection timed out: %w", err)
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return fmt.Errorf("Bluetooth permission denied: %w", err)
	case errors.Is(err, unix.EBUSY), errors.Is(err, unix.EALREADY), errors.Is(err, unix.EINPROGRESS), errors.Is(err, unix.EADDRINUSE):
		return fmt.Errorf("printer connection is busy: %w", err)
	case shouldReconnectBluetooth(err):
		return fmt.Errorf("printer is offline or out of range: %w", err)
	default:
		return fmt.Errorf("RFCOMM connection failed: %w", err)
	}
}
