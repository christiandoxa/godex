package main

import (
	"fmt"
	"testing"
	"time"
)

func TestFixtureExternalResponseIDsUseBindingRecorderNormalization04384(t *testing.T) {
	response := exchange{Status: 200, Body: `{"id":" chatcmpl-synthetic "}`}
	upstream := []upstreamRequest{{KeySlot: "single", AuthOK: true}}
	owners := fixtureContinuationOwners(response, upstream, "http://127.0.0.1:1/v1", nil)
	if len(owners) != 1 || owners["chatcmpl-synthetic"].kind != "previous" || owners[" chatcmpl-synthetic "].kind != "" {
		t.Fatal("external response owner fixture bypassed the pinned recorder's trim")
	}
	for _, id := range []string{"chatcmpl-synthetic", " chatcmpl-synthetic "} {
		state := fmt.Sprintf(`{"version":1,"bindings":[{"kind":"previous","key":%q,"account_id":%q,"updated_unix":%d}]}`, fixturePreviousDigest(id), fixtureConflictOwner, time.Now().Unix())
		if validFixtureRoutingBindings([]byte(state), owners) != (id == "chatcmpl-synthetic") {
			t.Fatal("owner audit accepted the wrong original or normalized identity")
		}
	}
	response.Status = 503
	if len(fixtureContinuationOwners(response, upstream, "http://127.0.0.1:1/v1", nil)) != 0 {
		t.Fatal("rejected response invented an owner")
	}
	response = exchange{Status: 200, Body: `{"id":" \t "}`}
	if len(fixtureContinuationOwners(response, upstream, "http://127.0.0.1:1/v1", nil)) != 0 {
		t.Fatal("blank observed response ID invented an owner")
	}
}
