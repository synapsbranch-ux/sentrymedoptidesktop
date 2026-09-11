//go:build linux

package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestParseBluetoothTargetReversesAddressBytes(t *testing.T) {
	target, err := parseBluetoothTarget(knownPT280Address, 1)
	if err != nil {
		t.Fatalf("valid destination rejected: %v", err)
	}
	// BlueZ sockets take the address least-significant byte first.
	if want := [6]uint8{0x27, 0x6E, 0x90, 0x33, 0x22, 0x10}; target != want {
		t.Fatalf("target=%#v, want %#v", target, want)
	}
	for _, invalid := range []struct {
		address string
		channel int
	}{{"", 1}, {"10:22:33:90:6E", 1}, {"ZZ:22:33:90:6E:27", 1}, {knownPT280Address, 0}, {knownPT280Address, 31}} {
		if _, err = parseBluetoothTarget(invalid.address, invalid.channel); err == nil {
			t.Fatalf("accepted invalid destination %q channel %d", invalid.address, invalid.channel)
		}
	}
}

// The codes are what the clinic sees in the print history, so each BlueZ
// failure has to arrive as the one the staff can act on.
func TestDescribeRFCOMMErrorReportsActionableCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"printer still holds the previous session", &rfcommError{stage: "connect", cause: unix.EBUSY}, "PRINTER_BUSY"},
		{"data link still closing", &rfcommError{stage: "connect", cause: unix.EADDRINUSE}, "PRINTER_BUSY"},
		{"paired but not authorized", &rfcommError{stage: "connect", cause: unix.EACCES}, "PRINTER_PERMISSION_DENIED"},
		{"computer forbids Bluetooth sockets", &rfcommError{stage: "create", cause: unix.EPERM}, "PRINTER_PERMISSION_DENIED"},
		{"no Bluetooth stack", &rfcommError{stage: "create", cause: unix.EAFNOSUPPORT}, "PRINTER_UNAVAILABLE"},
		{"switched off", &rfcommError{stage: "connect", cause: unix.EHOSTDOWN}, "PRINTER_OFFLINE"},
		{"out of range", &rfcommError{stage: "connect", cause: unix.ETIMEDOUT}, "PRINTER_OFFLINE"},
		{"never answered", &rfcommError{stage: "connect", cause: errPrinterDeadline}, "PRINTER_TIMEOUT"},
		{"request abandoned", context.DeadlineExceeded, "PRINTER_TIMEOUT"},
		{"link dropped mid receipt", &rfcommError{stage: "write", cause: unix.EPIPE}, "PRINTER_WRITE_FAILED"},
		{"unclassified", &rfcommError{stage: "bind", cause: unix.EINVAL}, "PRINTER_CONNECTION_FAILED"},
	}
	for _, item := range cases {
		described := describeRFCOMMError(item.err)
		if got := printerErrorCode(described); got != item.want {
			t.Fatalf("%s: code=%s, want %s", item.name, got, item.want)
		}
		if described.Error() == "" || printerCause(described) == "" {
			t.Fatalf("%s: message=%q cause=%q, want both populated", item.name, described.Error(), printerCause(described))
		}
		if !errors.Is(described, item.err) {
			t.Fatalf("%s: described error lost the underlying cause", item.name)
		}
	}
	if describeRFCOMMError(nil) != nil {
		t.Fatal("a successful print was reported as an error")
	}
}

// A busy link is the failure that fixes itself; permission and sleep are each
// repaired once, then reported.
func TestRFCOMMRecoveryRetriesBusyAndRepairsEachFaultOnce(t *testing.T) {
	busy := &rfcommError{stage: "connect", cause: unix.EBUSY}
	if got := rfcommRecovery(busy, false, false); got != recoveryWait {
		t.Fatalf("busy link: %s, want %s", got, recoveryWait)
	}
	if got := rfcommRecovery(busy, true, true); got != recoveryWait {
		t.Fatalf("busy link after repairs: %s, want %s", got, recoveryWait)
	}
	denied := &rfcommError{stage: "connect", cause: unix.EACCES}
	if got := rfcommRecovery(denied, false, false); got != recoveryAuthorize {
		t.Fatalf("unauthorized printer: %s, want %s", got, recoveryAuthorize)
	}
	if got := rfcommRecovery(denied, true, false); got != recoveryGiveUp {
		t.Fatalf("still unauthorized after trusting: %s, want %s", got, recoveryGiveUp)
	}
	if got := rfcommRecovery(&rfcommError{stage: "create", cause: unix.EACCES}, false, false); got != recoveryGiveUp {
		t.Fatalf("local socket refusal: %s, want %s", got, recoveryGiveUp)
	}
	asleep := &rfcommError{stage: "connect", cause: unix.ECONNREFUSED}
	if got := rfcommRecovery(asleep, false, false); got != recoveryWake {
		t.Fatalf("sleeping link: %s, want %s", got, recoveryWake)
	}
	if got := rfcommRecovery(asleep, false, true); got != recoveryGiveUp {
		t.Fatalf("still asleep after waking: %s, want %s", got, recoveryGiveUp)
	}
	// Half a receipt is already on the roll, so no failure during the send is
	// retried, however retryable the errno looks on its own.
	for _, cause := range []error{unix.EPIPE, unix.EBUSY, unix.ECONNRESET, unix.EACCES} {
		if got := rfcommRecovery(&rfcommError{stage: "write", cause: cause}, false, false); got != recoveryGiveUp {
			t.Fatalf("failure while sending (%v): %s, want %s", cause, got, recoveryGiveUp)
		}
	}
}

func TestSleepContextStopsWithTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if sleepContext(ctx, time.Hour) {
		t.Fatal("a cancelled print job kept waiting")
	}
}
