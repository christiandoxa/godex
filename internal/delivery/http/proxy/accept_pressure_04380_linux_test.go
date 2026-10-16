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
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

const proxyPressureChild04380 = "GODEX_04380_PROXY_PRESSURE_CHILD"
const proxyPressurePath04380 = "/backend-api/godex/responses"
const proxyPressureBody04380 = `{"model":"gpt-6","input":"synthetic ping"}`

type proxyPressureGateway04380 struct{ calls atomic.Int32 }

func (g *proxyPressureGateway04380) Execute(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	count := g.calls.Add(1)
	return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"call":%d}`, count)))}, nil
}

// Unlike a synthetic net.Listener error, this test consumes all file
// descriptors in an isolated child and forces the real TCP Accept() path to
// receive Linux EMFILE. The queued HTTP request must succeed after pressure
// is released, on exactly the original port, without a duplicate dispatch.
func TestProdex04380ProxyListenerSurvivesActualEMFILEAndQueuedRequest(t *testing.T) {
	if os.Getenv(proxyPressureChild04380) == "1" {
		if err := proxyPressureChildRun04380(); err != nil {
			t.Fatal(err)
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProdex04380ProxyListenerSurvivesActualEMFILEAndQueuedRequest$")
	command.Env = append(os.Environ(), proxyPressureChild04380+"=1")
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
	scanner := bufio.NewReader(stdout)
	phase := func(prefix string) string {
		t.Helper()
		answer, err := scanner.ReadString('\n')
		if err != nil || !strings.HasPrefix(answer, prefix) {
			t.Fatalf("child phase %q got %q err=%v stderr=%s", prefix, answer, err, stderr.String())
		}
		return strings.TrimSpace(strings.TrimPrefix(answer, prefix))
	}
	address := phase("LISTEN ")
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 6 * time.Second}
	post := func() string {
		t.Helper()
		response, err := client.Post(address+proxyPressurePath04380, "application/json", strings.NewReader(proxyPressureBody04380))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("proxy reply %d %q: %v", response.StatusCode, payload, err)
		}
		return string(payload)
	}
	if result := post(); result != `{"call":1}` {
		t.Fatalf("baseline proxy dispatch=%s", result)
	}
	_, _ = io.WriteString(stdin, "PRESSURE\n")
	phase("PRESSURE")
	connection, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, err = io.WriteString(connection, fmt.Sprintf("POST %s HTTP/1.1\r\nHost: local\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", proxyPressurePath04380, len(proxyPressureBody04380), proxyPressureBody04380))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		_ = connection.SetReadDeadline(time.Now().Add(7 * time.Second))
		response, readErr := http.ReadResponse(bufio.NewReader(connection), nil)
		if readErr != nil {
			done <- result{err: readErr}
			return
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		if readErr != nil || response.StatusCode != 200 {
			done <- result{err: fmt.Errorf("status=%d body=%s error=%v", response.StatusCode, body, readErr)}
			return
		}
		done <- result{value: string(body)}
	}()
	select {
	case observed := <-done:
		t.Fatalf("request escaped actual FD pressure: %+v", observed)
	case <-time.After(200 * time.Millisecond):
	}
	_, _ = io.WriteString(stdin, "RELEASE\n")
	phase("RELEASED")
	select {
	case observed := <-done:
		if observed.err != nil || observed.value != `{"call":2}` {
			t.Fatalf("queued request lost or duplicated: %+v", observed)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("queued request never recovered after EMFILE")
	}
	if got := post(); got != `{"call":3}` {
		t.Fatalf("new request lost or replayed: %s", got)
	}
	_, _ = io.WriteString(stdin, "DONE\n")
	if err := command.Wait(); err != nil {
		t.Fatalf("proxy child exited: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "too many open files") {
		t.Fatalf("listener never evidenced real accept EMFILE: %s", stderr.String())
	}
}

func proxyPressureChildRun04380() error {
	accounts := func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{{ID: "synthetic-fd", Home: "/synthetic", Enabled: true}}, nil
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{Accounts: accounts, Gateway: &proxyPressureGateway04380{}})
	if err != nil {
		return err
	}
	server, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		return err
	}
	if err = server.Start(); err != nil {
		return err
	}
	defer server.Close(context.Background())
	fmt.Println("LISTEN " + server.Endpoint())
	input := bufio.NewReader(os.Stdin)
	if _, err = input.ReadString('\n'); err != nil {
		return err
	}
	var limit syscall.Rlimit
	if err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return err
	}
	if limit.Cur < 96 {
		return fmt.Errorf("fd limit unexpectedly low: %d", limit.Cur)
	}
	limit.Cur = 96
	if err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return err
	}
	open := make([]*os.File, 0, 96)
	for {
		file, openErr := os.Open("/dev/null")
		if errors.Is(openErr, syscall.EMFILE) {
			break
		}
		if openErr != nil {
			return fmt.Errorf("fd pressure setup: %w", openErr)
		}
		open = append(open, file)
		if len(open) > 256 {
			return errors.New("descriptor limit was not enforced")
		}
	}
	fmt.Println("PRESSURE")
	if _, err = input.ReadString('\n'); err != nil {
		return err
	}
	for _, file := range open {
		_ = file.Close()
	}
	fmt.Println("RELEASED")
	_, err = input.ReadString('\n')
	return err
}
