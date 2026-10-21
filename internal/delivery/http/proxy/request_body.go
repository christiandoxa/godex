package proxy

import (
	"context"
	"errors"
	"io"
)

var errRequestBodyTooLarge = errors.New("request body limit exceeded")

func readLimited(ctx context.Context, reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(reader, limit+1))
		read <- result{data: data, err: err}
	}()
	select {
	case result := <-read:
		_ = reader.Close()
		if result.err != nil {
			return nil, result.err
		}
		if int64(len(result.data)) > limit {
			return nil, errRequestBodyTooLarge
		}
		return result.data, nil
	case <-ctx.Done():
		_ = reader.Close()
		return nil, ctx.Err()
	}
}
