package kiro

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	kiroStreamQueueCapacity      = 16
	kiroDefaultStreamIdleTimeout = 300 * time.Second
)

type kiroStreamChunk struct {
	data []byte
	err  error
}

type cancelStreamReader struct {
	reader *io.PipeReader
	cancel context.CancelCauseFunc
	once   sync.Once
}

func (reader *cancelStreamReader) Read(buffer []byte) (int, error) {
	return reader.reader.Read(buffer)
}

func (reader *cancelStreamReader) Close() error {
	reader.once.Do(func() { reader.cancel(context.Canceled) })
	return reader.reader.Close()
}

func (source *Source) liveACPStream(
	ctx context.Context,
	home string,
	request runtimeRequest,
	route runtimeRoute,
	requestID uint64,
	profileName string,
) (io.ReadCloser, error) {
	streamCtx, cancel := context.WithCancelCause(ctx)
	child, err := source.startACPChild(streamCtx, home, request.model, request.effort)
	if err != nil {
		cancel(err)
		return nil, err
	}
	if err := bootstrapACP(child.writer, home); err != nil {
		child.stop()
		cancel(err)
		return nil, err
	}

	chunks := make(chan kiroStreamChunk, kiroStreamQueueCapacity)
	pipeReader, pipeWriter := io.Pipe()
	reader := &cancelStreamReader{reader: pipeReader, cancel: cancel}
	state := newKiroLiveState(requestID, request.model, profileName)
	activity := make(chan struct{}, 1)
	notifyKiroStreamActivity(activity)

	go monitorKiroStreamIdle(streamCtx, cancel, activity, source.streamIdleTimeout())
	go consumeKiroStream(streamCtx, cancel, pipeWriter, chunks)
	go source.produceKiroStream(streamCtx, child, route, request, state, chunks, activity)
	return reader, nil
}

func consumeKiroStream(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	writer *io.PipeWriter,
	chunks <-chan kiroStreamChunk,
) {
	defer cancel(nil)
	for {
		select {
		case <-ctx.Done():
			closeKiroStreamForContext(ctx, writer)
			return
		case chunk, ok := <-chunks:
			if consumeKiroStreamChunk(ctx, writer, chunk, ok) {
				return
			}
		}
	}
}

func closeKiroStreamForContext(ctx context.Context, writer *io.PipeWriter) {
	cause := context.Cause(ctx)
	if cause == nil {
		cause = ctx.Err()
	}
	_ = writer.CloseWithError(cause)
}

func consumeKiroStreamChunk(ctx context.Context, writer *io.PipeWriter, chunk kiroStreamChunk, ok bool) bool {
	if !ok {
		if ctx.Err() != nil {
			closeKiroStreamForContext(ctx, writer)
		} else {
			_ = writer.Close()
		}
		return true
	}
	if chunk.err != nil {
		_ = writer.CloseWithError(chunk.err)
		return true
	}
	if len(chunk.data) == 0 {
		return false
	}
	_, err := writer.Write(chunk.data)
	return err != nil
}

type kiroStreamProducer struct {
	source    *Source
	ctx       context.Context
	child     *acpChild
	route     runtimeRoute
	state     *kiroLiveState
	request   runtimeRequest
	chunks    chan<- kiroStreamChunk
	collector acpCollector
	received  int
	started   bool
	activity  chan<- struct{}
}

func (source *Source) produceKiroStream(
	ctx context.Context,
	child *acpChild,
	route runtimeRoute,
	request runtimeRequest,
	state *kiroLiveState,
	chunks chan<- kiroStreamChunk,
	activity chan<- struct{},
) {
	defer close(chunks)
	defer child.stop()

	producer := &kiroStreamProducer{
		source: source, ctx: ctx, child: child, route: route, state: state, request: request, chunks: chunks,
		collector: acpCollector{prompt: request.prompt}, activity: activity,
	}
	producer.collector.onNotification = producer.observe
	if err := producer.run(); err != nil {
		sendKiroStreamError(ctx, chunks, err)
	}
}

func (producer *kiroStreamProducer) observe(envelope acpEnvelope) error {
	for _, data := range producer.state.observe(producer.route, envelope) {
		if err := sendKiroStreamChunk(producer.ctx, producer.chunks, data); err != nil {
			return err
		}
	}
	return nil
}

func (producer *kiroStreamProducer) run() error {
	for producer.child.scanner.Scan() {
		done, err := producer.acceptLine(producer.child.scanner.Bytes())
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return producer.finishScanner()
}

func (producer *kiroStreamProducer) acceptLine(line []byte) (bool, error) {
	if producer.ctx.Err() != nil {
		return true, nil
	}
	notifyKiroStreamActivity(producer.activity)
	producer.received += len(line) + 1
	if producer.received > acpBufferedOutputMaxBytes {
		return false, fmt.Errorf("Kiro ACP output exceeded safe size limit (%d)", acpBufferedOutputMaxBytes)
	}
	envelope, skip, err := decodeACPLine(append([]byte(nil), line...))
	if err != nil || skip {
		return false, err
	}
	wasPromptSent := producer.collector.promptSent
	done, err := producer.collector.accept(producer.child.writer, envelope)
	if err != nil {
		return false, err
	}
	if !wasPromptSent && producer.collector.promptSent && !producer.started {
		if err := sendKiroStreamChunk(producer.ctx, producer.chunks, producer.state.start(producer.route)); err != nil {
			return false, err
		}
		producer.started = true
	}
	if !done {
		return false, nil
	}
	turn, err := producer.collector.turn(producer.ctx)
	if err != nil {
		return false, err
	}
	response := kiroResponseFromTurn(
		turn, producer.state.requestID, producer.state.streamModel, producer.state.profileName,
	)
	response["created_at"] = producer.state.createdAt
	rememberKiroConversation(
		producer.source.conversations, producer.state.profileName, producer.request, response,
	)
	for _, data := range producer.state.finishResponse(producer.route, response) {
		if err := sendKiroStreamChunk(producer.ctx, producer.chunks, data); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (producer *kiroStreamProducer) finishScanner() error {
	if producer.ctx.Err() != nil {
		return nil
	}
	if err := producer.child.scanner.Err(); err != nil {
		return fmt.Errorf("read Kiro ACP stdout: %w", err)
	}
	return errors.New("Kiro ACP stream ended before prompt completion")
}

func sendKiroStreamChunk(ctx context.Context, chunks chan<- kiroStreamChunk, data []byte) error {
	copyData := append([]byte(nil), data...)
	select {
	case chunks <- kiroStreamChunk{data: copyData}:
		return nil
	case <-ctx.Done():
		return errors.New("Kiro ACP stream consumer disconnected")
	}
}

func sendKiroStreamError(ctx context.Context, chunks chan<- kiroStreamChunk, err error) {
	if err == nil || ctx.Err() != nil {
		return
	}
	select {
	case chunks <- kiroStreamChunk{err: err}:
	case <-ctx.Done():
	}
}

func notifyKiroStreamActivity(activity chan<- struct{}) {
	if activity == nil {
		return
	}
	select {
	case activity <- struct{}{}:
	default:
	}
}

func monitorKiroStreamIdle(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	activity <-chan struct{},
	timeout time.Duration,
) {
	if timeout <= 0 {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
			cancel(errors.New("Kiro ACP stream idle timeout"))
			return
		}
	}
}

func (source *Source) streamIdleTimeout() time.Duration {
	if source != nil && source.getenv != nil {
		for _, envName := range []string{"GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS"} {
			if raw := strings.TrimSpace(source.getenv(envName)); raw != "" {
				if milliseconds, err := strconv.ParseUint(raw, 10, 64); err == nil && milliseconds > 0 {
					return time.Duration(milliseconds) * time.Millisecond
				}
			}
		}
	}
	return kiroDefaultStreamIdleTimeout
}
