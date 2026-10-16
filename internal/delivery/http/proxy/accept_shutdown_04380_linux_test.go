//go:build linux

package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

const proxyShutdownPressureChild04380 = "GODEX_04380_PROXY_SHUTDOWN_PRESSURE"

// Check the other branch of the tagged Prodex 0.438.0 listener repair: an
// intentional shutdown must exit even when the OS is still returning EMFILE,
// with no extra upstream dispatch of queued-but-unaccepted HTTP work.
func TestProdex04380ProxyShutdownStopsAcceptRetryDuringEMFILE(t *testing.T) {
	if os.Getenv(proxyShutdownPressureChild04380) == "1" {
		if err := proxyShutdownPressureChildRun04380(); err != nil {
			t.Fatal(err)
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProdex04380ProxyShutdownStopsAcceptRetryDuringEMFILE$")
	command.Env = append(os.Environ(), proxyShutdownPressureChild04380+"=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "LISTEN ") {
		t.Fatalf("no proxy listener: %q %v %s", line, err, stderr.String())
	}
	address := strings.TrimSpace(strings.TrimPrefix(line, "LISTEN "))
	line, err = reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "PRESSURE" {
		t.Fatalf("EMFILE was not established: %q %v %s", line, err, stderr.String())
	}
	connection, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(connection, fmt.Sprintf("POST %s HTTP/1.1\r\nHost: synthetic\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", proxyPressurePath04380))
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	scratch := make([]byte, 1)
	if _, err = connection.Read(scratch); err == nil {
		t.Fatal("HTTP request dispatched while no descriptor was available")
	}
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("expected queued request to wait under EMFILE: %v", err)
	}
	if _, err = io.WriteString(stdin, "STOP\n"); err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	stopped := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		reply, readErr := reader.ReadString('\n')
		stopped <- struct {
			line string
			err  error
		}{reply, readErr}
	}()
	select {
	case reply := <-stopped:
		if reply.err != nil || strings.TrimSpace(reply.line) != "STOPPED" {
			t.Fatalf("proxy refused clean shutdown under EMFILE: %+v stderr=%s", reply, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("temporary accept retry ignored shutdown")
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("proxy child shutdown failed: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "too many open files") {
		t.Fatalf("no real listener EMFILE was observed: %s", stderr.String())
	}
	next, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), 250*time.Millisecond)
	if err == nil {
		next.Close()
		t.Fatal("proxy port remained open after shutdown")
	}
}

func proxyShutdownPressureChildRun04380() error {
	gateway := &proxyPressureGateway04380{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-fd", Home: "/synthetic", Enabled: true}}, nil
		}, Gateway: gateway,
	})
	if err != nil {
		return err
	}
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		return err
	}
	if err := proxy.Start(); err != nil {
		return err
	}
	fmt.Println("LISTEN " + proxy.Endpoint())
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return err
	}
	if limit.Cur < 96 {
		return fmt.Errorf("unexpected low file descriptor limit: %d", limit.Cur)
	}
	limit.Cur = 96
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return err
	}
	pressure := make([]*os.File, 0, 96)
	for {
		file, openErr := os.Open("/dev/null")
		if errors.Is(openErr, syscall.EMFILE) {
			break
		}
		if openErr != nil {
			return openErr
		}
		pressure = append(pressure, file)
		if len(pressure) > 256 {
			return errors.New("fd limit did not take effect")
		}
	}
	fmt.Println("PRESSURE")
	reader := bufio.NewReader(os.Stdin)
	if _, err := reader.ReadString('\n'); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = proxy.Close(ctx)
	for _, file := range pressure {
		_ = file.Close()
	}
	if err != nil {
		return err
	}
	if gateway.calls.Load() != 0 {
		return fmt.Errorf("queued unaccepted inference was dispatched %d times", gateway.calls.Load())
	}
	fmt.Println("STOPPED")
	return nil
}
