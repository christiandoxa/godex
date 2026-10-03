package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type activityRecorder interface {
	Record(context.Context, runtimemodel.Event) error
}

type requestActivity struct {
	id        string
	sequence  uint64
	method    string
	path      string
	accountID string
	status    int
	message   string
	failed    bool
	started   time.Time
}

func (proxy *Proxy) startActivity(request *http.Request) *requestActivity {
	started := time.Now()
	sequence := proxy.sequence.Add(1)
	activity := &requestActivity{
		id:       fmt.Sprintf("%d-%d", started.UnixNano(), sequence),
		sequence: sequence,
		method:   request.Method,
		path:     request.URL.Path,
		started:  started,
	}
	proxy.recordActivity(context.WithoutCancel(request.Context()), runtimemodel.Event{
		Kind: "request_started", RequestID: activity.id, Method: activity.method, Path: activity.path,
	})
	return activity
}

func (activity *requestActivity) fail(status int, message string) {
	activity.failed = true
	activity.status = status
	activity.message = message
}

func (activity *requestActivity) upstream(accountID string, status int) {
	activity.accountID = accountID
	activity.status = status
}

func (proxy *Proxy) finishActivity(ctx context.Context, activity *requestActivity) {
	if activity == nil {
		return
	}
	kind := "request_completed"
	if activity.failed {
		kind = "request_failed"
	}
	proxy.recordActivity(ctx, runtimemodel.Event{
		Kind: kind, RequestID: activity.id, Method: activity.method, Path: activity.path,
		AccountID: activity.accountID, StatusCode: activity.status,
		DurationMillis: time.Since(activity.started).Milliseconds(), Message: activity.message,
	})
}

func (proxy *Proxy) recordActivity(ctx context.Context, event runtimemodel.Event) {
	if proxy.activity != nil {
		_ = proxy.activity.Record(ctx, event)
	}
}

func (activity *requestActivity) finishLifecycle(lifecycle *requestLifecycle) {
	switch lifecycle.phase {
	case requestCompleted:
		return
	case requestFailedAfterCommit:
		activity.fail(activity.status, "upstream response failed after commitment")
	default:
		activity.fail(http.StatusBadGateway, "upstream response failed before commitment")
	}
}
