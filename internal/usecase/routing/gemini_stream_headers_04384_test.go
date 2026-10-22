package routing

import (
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type geminiHeaderReadProbe04384 struct{ reads atomic.Int32 }

func (reader *geminiHeaderReadProbe04384) Read([]byte) (int, error) {
	reader.reads.Add(1)
	return 0, io.EOF
}

func TestProdex04384GeminiSSECommitsHeadersWithoutBodyInspection(t *testing.T) {
	for _, fixture := range []struct {
		kind   string
		direct bool
	}{
		{"gemini", true},
		{" GEMINI ", true},
		{"openai", false},
		{"deepseek", false},
		{"claude", false},
	} {
		t.Run(fixture.kind, func(t *testing.T) {
			probe := &geminiHeaderReadProbe04384{}
			response := &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: io.NopCloser(probe)}
			router := &Router{now: time.Now}
			outcome, pending, err, handled := router.classifySpecialResponse(response, &pendingResponse{}, fixture.kind)
			defer response.Body.Close()
			if !handled {
				t.Fatal("SSE response did not retain its routing policy")
			}
			if fixture.direct {
				if err != nil || probe.reads.Load() != 0 || outcome.kind != responsePass || len(pending.prefix) != 0 {
					t.Fatalf("Gemini headers waited on response bytes: reads=%d error=%v", probe.reads.Load(), err)
				}
			} else if probe.reads.Load() == 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("other provider lost precommit inspection: reads=%d error=%v", probe.reads.Load(), err)
			}
		})
	}
}
