//go:build linux

package server

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// A PT280 answers one RFCOMM connection at a time and BlueZ needs a moment
	// to tear the previous session down, so an attempt that finds the link busy
	// is retried rather than reported to the cashier.
	rfcommAttempts       = 4
	rfcommConnectTimeout = 12 * time.Second
	rfcommWriteTimeout   = 15 * time.Second
	rfcommDrainTimeout   = 10 * time.Second
	// A mobile thermal head is still pulling paper when the last byte leaves the
	// socket. Dropping RFCOMM at that instant truncates the end of the receipt,
	// so the link is held open briefly after the queue empties.
	rfcommSettleDelay   = 300 * time.Millisecond
	bluetoothctlTimeout = 6 * time.Second
)

// errPrinterDeadline separates "we stopped waiting" from the kernel's own
// ETIMEDOUT, which on a Bluetooth link means the device stopped answering.
var errPrinterDeadline = errors.New("the printer did not answer before the deadline")

// rfcommError records which step of the session failed, so a refusal to open a
// Bluetooth socket at all is not reported as a refusal by the printer.
type rfcommError struct {
	stage string
	cause error
}

func (e *rfcommError) Error() string { return e.stage + " RFCOMM socket: " + e.cause.Error() }
func (e *rfcommError) Unwrap() error { return e.cause }

func rfcommStageOf(err error) string {
	var staged *rfcommError
	if errors.As(err, &staged) {
		return staged.stage
	}
	return ""
}

func writeBluetoothRFCOMM(ctx context.Context, address string, channel int, payload []byte) error {
	target, err := parseBluetoothTarget(address, channel)
	if err != nil {
		return err
	}
	var attemptErr error
	authorized, woken := false, false
	for attempt := 0; attempt < rfcommAttempts; attempt++ {
		if ctx.Err() != nil {
			return describeRFCOMMError(ctx.Err())
		}
		attemptErr = writeRFCOMMOnce(ctx, target, channel, payload)
		if attemptErr == nil {
			return nil
		}
		switch rfcommRecovery(attemptErr, authorized, woken) {
		case recoveryWait:
			// The previous data link is still closing inside BlueZ, or the
			// printer has not released the last session. There is nothing to
			// repair: let the link settle and open a fresh socket.
			if !sleepContext(ctx, time.Duration(attempt+1)*600*time.Millisecond) {
				return describeRFCOMMError(attemptErr)
			}
		case recoveryAuthorize:
			authorized = true
			// Paired but not authorized: bluetoothd asks a registered agent to
			// approve the Serial Port service and a clinic server has no agent
			// to answer, so the kernel returns EACCES. Trusting the one printer
			// the administrator configured is the documented remedy; the
			// address was validated before it reached this argv.
			runBluetoothctl(ctx, "trust", address)
			runBluetoothctl(ctx, "connect", address)
		case recoveryWake:
			woken = true
			// BlueZ may know the paired device while its ACL link is asleep.
			// Wake it once, then open a fresh RFCOMM socket.
			runBluetoothctl(ctx, "connect", address)
		default:
			return describeRFCOMMError(attemptErr)
		}
	}
	return describeRFCOMMError(attemptErr)
}

// Recovery actions for a failed attempt. Each repair runs at most once per job:
// a printer that stays unauthorized or asleep after one attempt to fix it is
// reported to the clinic rather than hammered.
const (
	recoveryGiveUp    = "give-up"
	recoveryWait      = "wait"
	recoveryAuthorize = "authorize"
	recoveryWake      = "wake"
)

func rfcommRecovery(err error, authorized, woken bool) string {
	// Once bytes have left for the printer, part of the receipt may already be
	// on the roll. Resending it would print that part twice, so a failure while
	// sending is always reported and the clinic decides whether to reprint.
	if rfcommStageOf(err) == "write" {
		return recoveryGiveUp
	}
	switch {
	case busyRFCOMM(err):
		return recoveryWait
	case deniedRFCOMM(err):
		// A refusal to create the socket at all comes from this computer, not
		// from the printer, so pairing it differently cannot help.
		if authorized || rfcommStageOf(err) == "create" {
			return recoveryGiveUp
		}
		return recoveryAuthorize
	case shouldReconnectBluetooth(err) && !woken:
		return recoveryWake
	default:
		return recoveryGiveUp
	}
}

func parseBluetoothTarget(address string, channel int) ([6]uint8, error) {
	var target [6]uint8
	if !bluetoothAddress.MatchString(address) || channel < 1 || channel > 30 {
		return target, printerFailure("PRINTER_UNAVAILABLE", "The saved printer configuration is invalid. Select the printer again under System \u2192 Printers.", nil)
	}
	for index, part := range strings.Split(address, ":") {
		value, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return target, printerFailure("PRINTER_UNAVAILABLE", "The saved printer configuration is invalid. Select the printer again under System \u2192 Printers.", err)
		}
		target[5-index] = uint8(value)
	}
	return target, nil
}

// writeRFCOMMOnce opens one RFCOMM session, sends the receipt and waits for the
// kernel to hand the bytes to the controller before closing.
func writeRFCOMMOnce(ctx context.Context, target [6]uint8, channel int, payload []byte) error {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.BTPROTO_RFCOMM)
	if err != nil {
		return &rfcommError{stage: "create", cause: err}
	}
	defer func() { _ = unix.Close(fd) }()
	// Match BlueZ's rfcomm client: select a local adapter/channel before the
	// remote connect. Some controllers reject an implicit source selection.
	if err = unix.Bind(fd, &unix.SockaddrRFCOMM{}); err != nil {
		return &rfcommError{stage: "bind", cause: err}
	}
	if err = connectRFCOMM(ctx, fd, target, channel); err != nil {
		return err
	}
	if err = writeRFCOMMPayload(ctx, fd, payload); err != nil {
		return err
	}
	drainRFCOMM(ctx, fd)
	return nil
}

// connectRFCOMM connects without blocking the request for the kernel's own
// RFCOMM timeout, which can outlast the cashier's patience by minutes.
func connectRFCOMM(ctx context.Context, fd int, target [6]uint8, channel int) error {
	err := unix.Connect(fd, &unix.SockaddrRFCOMM{Addr: target, Channel: uint8(channel)})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EINPROGRESS), errors.Is(err, unix.EALREADY), errors.Is(err, unix.EINTR):
	default:
		return &rfcommError{stage: "connect", cause: err}
	}
	if err = waitForSocket(ctx, fd, unix.POLLOUT, rfcommConnectTimeout); err != nil {
		return &rfcommError{stage: "connect", cause: err}
	}
	code, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
	if err != nil {
		return &rfcommError{stage: "connect", cause: err}
	}
	if code != 0 {
		return &rfcommError{stage: "connect", cause: unix.Errno(code)}
	}
	return nil
}

func writeRFCOMMPayload(ctx context.Context, fd int, payload []byte) error {
	for len(payload) > 0 {
		written, err := unix.Write(fd, payload)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EWOULDBLOCK):
			if waitErr := waitForSocket(ctx, fd, unix.POLLOUT, rfcommWriteTimeout); waitErr != nil {
				return &rfcommError{stage: "write", cause: waitErr}
			}
			continue
		case err != nil:
			return &rfcommError{stage: "write", cause: err}
		case written <= 0:
			return &rfcommError{stage: "write", cause: unix.EPIPE}
		}
		payload = payload[written:]
	}
	return nil
}

// drainRFCOMM waits for the outgoing queue to empty. write() only queues bytes;
// closing the socket while the queue is full loses the tail of the receipt.
func drainRFCOMM(ctx context.Context, fd int) {
	deadline := time.Now().Add(rfcommDrainTimeout)
	for time.Now().Before(deadline) {
		pending, err := unix.IoctlGetInt(fd, unix.TIOCOUTQ)
		if err != nil || pending == 0 {
			break
		}
		if !sleepContext(ctx, 50*time.Millisecond) {
			return
		}
	}
	sleepContext(ctx, rfcommSettleDelay)
}

func waitForSocket(ctx context.Context, fd int, events int16, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errPrinterDeadline
		}
		if remaining > 250*time.Millisecond {
			remaining = 250 * time.Millisecond
		}
		descriptors := []unix.PollFd{{Fd: int32(fd), Events: events}}
		ready, err := unix.Poll(descriptors, int(remaining.Milliseconds()))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		// Any reported event ends the wait; the caller reads SO_ERROR or the
		// next write() for the actual outcome.
		if ready > 0 {
			return nil
		}
	}
}

func runBluetoothctl(ctx context.Context, arguments ...string) {
	commandCtx, cancel := context.WithTimeout(ctx, bluetoothctlTimeout)
	defer cancel()
	// Fixed argv, never a shell, and only for the validated configured address.
	_ = exec.CommandContext(commandCtx, "bluetoothctl", arguments...).Run()
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// busyRFCOMM reports the one failure worth retrying by itself: the printer, or
// the kernel data link for its channel, is still held by the previous receipt.
func busyRFCOMM(err error) bool {
	return errors.Is(err, unix.EBUSY) || errors.Is(err, unix.EALREADY) || errors.Is(err, unix.EINPROGRESS) ||
		errors.Is(err, unix.EADDRINUSE) || errors.Is(err, unix.EBADFD)
}

func deniedRFCOMM(err error) bool {
	return errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM)
}

func shouldReconnectBluetooth(err error) bool {
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.EHOSTDOWN) || errors.Is(err, unix.EHOSTUNREACH) ||
		errors.Is(err, unix.ENETDOWN) || errors.Is(err, unix.ENETUNREACH) || errors.Is(err, unix.ETIMEDOUT) ||
		errors.Is(err, unix.ECONNRESET)
}

func describeRFCOMMError(err error) error {
	if err == nil {
		return nil
	}
	stage := rfcommStageOf(err)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, errPrinterDeadline):
		return printerFailure("PRINTER_TIMEOUT", "The printer did not answer in time. Check that it is switched on, has paper and is within Bluetooth range.", err)
	case errors.Is(err, unix.EAFNOSUPPORT), errors.Is(err, unix.EPROTONOSUPPORT):
		return printerFailure("PRINTER_UNAVAILABLE", "This computer has no usable Bluetooth stack. Enable Bluetooth and make sure BlueZ is running.", err)
	case stage == "create" && deniedRFCOMM(err):
		return printerFailure("PRINTER_PERMISSION_DENIED", "This computer does not allow the clinic server to open Bluetooth connections. Run it as the standard desktop account that owns the Bluetooth adapter, outside any sandbox.", err)
	case deniedRFCOMM(err):
		return printerFailure("PRINTER_PERMISSION_DENIED", "The printer refused the connection. Pair it (PIN 0000) and mark it trusted in Bluetooth settings, then print again.", err)
	case busyRFCOMM(err):
		return printerFailure("PRINTER_BUSY", "The printer accepts one connection at a time and is still busy. Wait a few seconds, then print again; if it persists, close any other program or /dev/rfcomm binding holding the printer.", err)
	case shouldReconnectBluetooth(err):
		return printerFailure("PRINTER_OFFLINE", "The printer is switched off or out of range. Switch it on, keep it near the clinic computer, then print again.", err)
	case stage == "write":
		return printerFailure("PRINTER_WRITE_FAILED", "The printer dropped the connection while the receipt was being sent. Check the paper roll and print again.", err)
	default:
		return printerFailure("PRINTER_CONNECTION_FAILED", "Could not open the connection to the printer.", err)
	}
}
