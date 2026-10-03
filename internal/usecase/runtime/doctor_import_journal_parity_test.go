package runtime

import (
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type importJournalDoctorAccounts struct{}

func (importJournalDoctorAccounts) Prepare() error { return nil }
func (importJournalDoctorAccounts) Root() string   { return "/godex" }
func (importJournalDoctorAccounts) List(context.Context) ([]accountentity.Account, error) {
	return nil, nil
}

type importJournalDoctorCodex struct{ calls int }

func (codex *importJournalDoctorCodex) Version(context.Context) (string, error) {
	codex.calls++
	return "codex 0.160.0", nil
}
func (*importJournalDoctorCodex) CheckProxySupport(context.Context) error { return nil }

type importJournalRepairer struct {
	count     int
	repaired  int
	countErr  error
	repairErr error
}

func (repairer *importJournalRepairer) CountImportAuthJournals(context.Context) (int, error) {
	return repairer.count, repairer.countErr
}
func (repairer *importJournalRepairer) RepairImportAuthJournals(context.Context) (int, error) {
	return repairer.repaired, repairer.repairErr
}

func TestDoctorImportJournalStatusMatchesProdexAfterRepair(t *testing.T) {
	codex := &importJournalDoctorCodex{}
	doctor := NewDoctor(importJournalDoctorAccounts{}, codex)
	doctor.SetImportJournalRepairer(&importJournalRepairer{count: 0, repaired: 2})
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{RepairImportAuthJournals: true})
	if err != nil {
		t.Fatal(err)
	}
	status := report.ImportAuthJournals
	if status == nil || status.OrphanCount != 0 || !status.RepairPerformed || status.Repaired != 2 || status.Status != "ok" {
		t.Fatalf("import journal status = %#v", status)
	}
	if codex.calls != 1 {
		t.Fatalf("baseline doctor calls = %d", codex.calls)
	}
}

func TestDoctorImportJournalStatusWarnsWithoutRepair(t *testing.T) {
	doctor := NewDoctor(importJournalDoctorAccounts{}, &importJournalDoctorCodex{})
	doctor.SetImportJournalRepairer(&importJournalRepairer{count: 1})
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status := report.ImportAuthJournals
	if status == nil || status.OrphanCount != 1 || status.RepairPerformed || status.Repaired != 0 || status.Status != "warning" {
		t.Fatalf("import journal status = %#v", status)
	}
}

func TestDoctorImportJournalRepairFailsBeforeBaselineDiagnostics(t *testing.T) {
	want := errors.New("synthetic repair failure")
	codex := &importJournalDoctorCodex{}
	doctor := NewDoctor(importJournalDoctorAccounts{}, codex)
	doctor.SetImportJournalRepairer(&importJournalRepairer{repairErr: want})
	_, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{RepairImportAuthJournals: true})
	if !errors.Is(err, want) || codex.calls != 0 {
		t.Fatalf("repair error/codex calls = %v / %d", err, codex.calls)
	}
}
