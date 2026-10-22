package deepseek

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384RuntimeBufferedResponseBodyLimit(t *testing.T) {
	const limit = 4 << 20
	const prefix, suffix = `{"id":"resp-body-limit","created":1,"choices":[{"message":{"content":"ok"}}],"padding":"`, `"}`
	for _, size := range []int{limit - 1, limit, limit + 1} {
		body := prefix + strings.Repeat(" ", size-len(prefix)-len(suffix)) + suffix
		result, err := translateResponseWithMetadata(&http.Response{StatusCode: 200,
			Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil)
		if result != nil {
			_ = result.Body.Close()
		}
		if size <= limit {
			if err != nil || result == nil || result.StatusCode != 200 {
				t.Fatalf("body boundary %d was rejected: %v", size, err)
			}
		} else if err == nil || result != nil {
			t.Fatalf("body above 4 MiB reached translation: err=%v response=%v", err, result != nil)
		}
	}
}
