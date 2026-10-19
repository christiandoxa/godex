//go:build linux || darwin

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// Measure raw accept semantics; net.Listener explicitly changes accepted flags.
func realtimeSocketMode04383(output io.Writer) error {
	listener, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("create realtime socket probe: %w", err)
	}
	defer unix.Close(listener)
	if err := unix.Bind(listener, &unix.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		return fmt.Errorf("bind realtime socket probe: %w", err)
	}
	if err := unix.Listen(listener, 1); err != nil {
		return fmt.Errorf("listen for realtime socket probe: %w", err)
	}
	if err := unix.SetNonblock(listener, true); err != nil {
		return fmt.Errorf("set realtime probe listener nonblocking: %w", err)
	}
	address, err := unix.Getsockname(listener)
	if err != nil {
		return fmt.Errorf("read realtime probe listener address: %w", err)
	}
	bound, ok := address.(*unix.SockaddrInet4)
	if !ok {
		return errors.New("realtime socket probe did not bind IPv4")
	}
	client, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(bound.Port)), time.Second)
	if err != nil {
		return fmt.Errorf("connect realtime socket probe: %w", err)
	}
	defer client.Close()
	ready, err := unix.Poll([]unix.PollFd{{Fd: int32(listener), Events: unix.POLLIN}}, 1000)
	if err != nil {
		return fmt.Errorf("wait for realtime socket probe connection: %w", err)
	}
	if ready != 1 {
		return errors.New("realtime socket probe connection was not ready")
	}
	accepted, _, err := unix.Accept(listener)
	if err != nil {
		return fmt.Errorf("accept realtime socket probe: %w", err)
	}
	defer unix.Close(accepted)
	flags, err := unix.FcntlInt(uintptr(accepted), unix.F_GETFL, 0)
	if err != nil {
		return fmt.Errorf("read accepted realtime socket flags: %w", err)
	}
	if err := unix.SetsockoptTimeval(accepted, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Usec: 20_000}); err != nil {
		return fmt.Errorf("set realtime socket probe read timeout: %w", err)
	}
	started := time.Now()
	_, err = unix.Read(accepted, make([]byte, 1))
	if err == nil {
		return errors.New("realtime socket probe idle read unexpectedly succeeded")
	}
	if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK) {
		return fmt.Errorf("read idle realtime socket probe: %w", err)
	}
	_, err = fmt.Fprintf(output, "realtime raw accept diagnostic: measured=true listener_nonblocking=true accepted_nonblocking=%t idle_read=would_block read_wait_us=%d\n", flags&unix.O_NONBLOCK != 0, time.Since(started).Microseconds())
	return err
}
