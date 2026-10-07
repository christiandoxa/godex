package subagent

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

type relayResult struct {
	bytes uint64
	err   error
}

type childOutputRelay struct {
	stdoutReader *os.File
	stdoutWriter *os.File
	stderrReader *os.File
	stderrWriter *os.File
	stdoutTarget io.Writer
	stderrTarget io.Writer
	stdoutResult chan relayResult
	stderrResult chan relayResult
}

func newChildOutputRelay(command *exec.Cmd, stdout, stderr io.Writer) (*childOutputRelay, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return nil, err
	}
	relay := &childOutputRelay{
		stdoutReader: stdoutReader,
		stdoutWriter: stdoutWriter,
		stderrReader: stderrReader,
		stderrWriter: stderrWriter,
		stdoutTarget: stdout,
		stderrTarget: stderr,
		stdoutResult: make(chan relayResult, 1),
		stderrResult: make(chan relayResult, 1),
	}
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter
	return relay, nil
}

func (relay *childOutputRelay) childStarted() {
	_ = relay.stdoutWriter.Close()
	_ = relay.stderrWriter.Close()
	relay.stdoutWriter = nil
	relay.stderrWriter = nil
	go relayChildOutput(relay.stdoutReader, relay.stdoutTarget, relay.stdoutResult)
	go relayChildOutput(relay.stderrReader, relay.stderrTarget, relay.stderrResult)
}

func (relay *childOutputRelay) close() {
	for _, file := range []*os.File{
		relay.stdoutReader, relay.stdoutWriter, relay.stderrReader, relay.stderrWriter,
	} {
		if file != nil {
			_ = file.Close()
		}
	}
}

func (relay *childOutputRelay) drain() (uint64, bool) {
	return drainRelayResults(relay.stdoutResult, relay.stderrResult)
}

func relayChildOutput(reader io.Reader, writer io.Writer, result chan<- relayResult) {
	var total uint64
	var writeErr error
	buffer := make([]byte, 8192)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			total += uint64(count)
			if writeErr == nil {
				if _, currentErr := writer.Write(buffer[:count]); currentErr != nil {
					writeErr = currentErr
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) && writeErr == nil {
				writeErr = err
			}
			result <- relayResult{bytes: total, err: writeErr}
			return
		}
	}
}

func drainRelayResults(stdout, stderr <-chan relayResult) (uint64, bool) {
	timer := time.NewTimer(outputDrainTimeout)
	defer timer.Stop()
	var total uint64
	incomplete := false
	for count := 0; count < 2; count++ {
		select {
		case result := <-stdout:
			total += result.bytes
			incomplete = incomplete || result.err != nil
			stdout = nil
		case result := <-stderr:
			total += result.bytes
			incomplete = incomplete || result.err != nil
			stderr = nil
		case <-timer.C:
			return total, true
		}
	}
	return total, incomplete
}
