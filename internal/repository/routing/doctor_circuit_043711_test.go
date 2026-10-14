package routing

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestProdex043711DoctorCircuitInspectionIsReadOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(1_700_000_000, 0)
	expired := routingentity.RouteCircuit{
		AccountID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Route: "responses",
		UntilUnix: now.Unix() - 1, StageUpdatedUnix: now.Unix() - 30,
	}
	stillOpen := routingentity.RouteCircuit{
		AccountID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Route: "websocket",
		UntilUnix: now.Unix() + 20, StageUpdatedUnix: now.Unix() - 20,
	}
	if err := store.prepare(); err != nil {
		t.Fatal(err)
	}
	if err := store.writeRouteCircuits([]routingentity.RouteCircuit{expired, stillOpen}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, "route-circuits.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := store.InspectRouteCircuits(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected) != 2 || inspected[0] != expired || inspected[1] != stillOpen {
		t.Fatalf("doctor inspection lost half-open route: %+v", inspected)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("read-only doctor mutated persisted circuit snapshot")
	}
	if _, err := store.LoadRouteCircuits(t.Context(), now, nil); err != nil {
		t.Fatal(err)
	}
	// The ordinary routing loader may clear an expired probe on restart;
	// that routing mutation remains separate from diagnostic inspection.
	inspected, err = store.InspectRouteCircuits(t.Context(), now)
	if err != nil || len(inspected) != 1 || inspected[0].AccountID != stillOpen.AccountID {
		t.Fatalf("diagnostic circuit reader did not reflect routing mutation: %+v %v", inspected, err)
	}
}
