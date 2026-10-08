package runtime

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

const prodex04360Session = "00000000-0000-4000-8000-000000000001"

func TestProdex04360CyberAccessProgramGlobalResumeSelector(t *testing.T) {
	for _, program := range []string{"standard", "daybreak_blue", "daybreak_red"} {
		cases := []struct {
			name      string
			args      []string
			wantIndex int
		}{
			{"before exec", []string{"--cyber-access-program", program, "exec", "resume", prodex04360Session, "continue"}, 4},
			{"before nested resume", []string{"exec", "--cyber-access-program", program, "resume", prodex04360Session, "continue"}, 4},
			{"before session", []string{"exec", "resume", "--cyber-access-program", program, prodex04360Session, "continue"}, 4},
			{"after session", []string{"exec", "resume", prodex04360Session, "--cyber-access-program", program, "continue"}, 2},
			{"inline program", []string{"exec", "--cyber-access-program=" + program, "resume", prodex04360Session, "--", "literal --cyber-access-program text"}, 3},
		}
		for _, tc := range cases {
			t.Run(program+"/"+tc.name, func(t *testing.T) {
				idx, actual := sessionArgument(tc.args)
				if idx != tc.wantIndex || !reflect.DeepEqual(actual, tc.args) {
					t.Fatalf("sessionArgument %#v => index %d, args %#v, want index %d + unchanged", tc.args, idx, actual, tc.wantIndex)
				}
				if !codexResumeRequested(tc.args) {
					t.Fatalf("failed to recognize resume with Cyber option: %#v", tc.args)
				}
			})
		}
	}
}

func TestProdex04360CyberProgramMustNotBecomeSessionOrPrompt(t *testing.T) {
	for _, args := range [][]string{
		{"exec", "resume", "--cyber-access-program"},
		{"exec", "resume", "--cyber-access-program", prodex04360Session},
		{"exec", "resume", "--", "--cyber-access-program", prodex04360Session},
		{"exec", "resume", "--", prodex04360Session},
		{"resume", "--", prodex04360Session},
		{"exec", "--cyber-access-program", "daybreak_blue", "review", "--uncommitted"},
	} {
		idx, got := sessionArgument(args)
		if idx != -1 || !reflect.DeepEqual(got, args) {
			t.Fatalf("Cyber option confused with session: %#v => %d/%#v", args, idx, got)
		}
	}
}

func TestProdex04360CyberProgramPassesRunAndSuperWithoutDaybreakOptIn(t *testing.T) {
	tail := []string{"exec", "--cyber-access-program", "daybreak_red", "resume", prodex04360Session, "continue"}
	_, runArgs, err := parseRunArguments(tail)
	if err != nil || !reflect.DeepEqual(runArgs, tail) {
		t.Fatalf("run argv=%v err=%v", runArgs, err)
	}
	options, err := parseSuperArguments(append([]string{"--no-presidio", "--no-sub-agent"}, tail...))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(options.codexArgs, "--cyber-access-program") ||
		!slices.Contains(options.codexArgs, "daybreak_red") ||
		!slices.Contains(options.codexArgs, prodex04360Session) {
		t.Fatalf("super lost native arguments: %#v", options.codexArgs)
	}
	if idx, args := sessionArgument(options.codexArgs); idx < 0 || args[idx] != prodex04360Session {
		t.Fatalf("super resume selector lost with prepended config: index %d, argv=%#v", idx, args)
	}
	for _, argument := range options.codexArgs {
		if argument == "--enable" || argument == "cli_daybreak" || argument == "daybreak=true" {
			t.Fatalf("Super implicitly enabled Daybreak: %#v", options.codexArgs)
		}
	}
}

func TestProdex04360ExplicitSolAndUltraStayUnchanged(t *testing.T) {
	args := []string{"--model", "gpt-6.1-sol", "-c", `model_reasoning_effort="ultra"`, "exec", "review"}
	options, err := parseSuperArguments(append([]string{"--no-presidio", "--no-sub-agent"}, args...))
	if err != nil {
		t.Fatal(err)
	}
	values := options.codexArgs
	// Super consumes --model and materializes the explicit model as one -c
	// assignment. Neither model nor reasoning effort may be changed.
	if !slices.Contains(values, `model="gpt-6.1-sol"`) ||
		!slices.Contains(values, `model_reasoning_effort="ultra"`) {
		t.Fatalf("explicit Sol/Ultra settings lost: %#v", values)
	}
	if slices.Contains(values, "cli_daybreak") || slices.Contains(values, "daybreak=true") {
		t.Fatalf("Daybreak activated without opt-in: %#v", values)
	}
}

// This exercises the run/Super parser -> native selector -> session catalogue
// boundary with the actual argv, instead of testing the individual walkers
// separately. Replacing a selected ID must not replace its Cyber program.
func TestProdex04360CyberProgramSurvivesNativeSessionResolution(t *testing.T) {
	sessions := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, resumeSessionReader{
		values: []sessionentity.Session{{
			ID: prodex04360Session, UpdatedUnix: 10,
			Path: "/shared/sessions/0.436.0.jsonl",
		}},
	}, nil)
	for _, program := range []string{"standard", "daybreak_blue", "daybreak_red"} {
		for _, args := range [][]string{
			{"exec", "--cyber-access-program", program, "resume", prodex04360Session, "continue"},
			{"exec", "resume", "--cyber-access-program", program, prodex04360Session, "continue"},
			{"exec", "resume", prodex04360Session, "--cyber-access-program", program, "continue"},
		} {
			_, native, err := parseRunArguments(args)
			if err != nil {
				t.Fatal(err)
			}
			for _, planned := range [][]string{native, func() []string {
				options, err := parseSuperArguments(append([]string{"--no-presidio", "--no-sub-agent"}, args...))
				if err != nil {
					t.Fatal(err)
				}
				return options.codexArgs
			}()} {
				index, original := sessionArgument(planned)
				if index < 0 || original[index] != prodex04360Session {
					t.Fatalf("native ID not selected: %#v, position %d", planned, index)
				}
				_, resolved, err := sessions.ResolveArguments(context.Background(), sessionmodel.Launch{
					SessionSelector: original[index], IDIndex: index, Arguments: original,
				})
				if err != nil {
					t.Fatalf("session resolution lost Cyber native argv: %v, %#v", err, planned)
				}
				pair := []string{"--cyber-access-program", program}
				at := slices.Index(resolved, pair[0])
				if at < 0 || at+1 >= len(resolved) || !reflect.DeepEqual(resolved[at:at+2], pair) {
					t.Fatalf("resolved argv split Cyber option/value: %#v", resolved)
				}
				if resolved[index] != prodex04360Session || resolved[len(resolved)-1] != "continue" {
					t.Fatalf("resolved session/prompt shifted: %#v", resolved)
				}
			}
		}
	}
}

func TestProdex04360DaybreakExplicitOrderingIsPreserved(t *testing.T) {
	tail := []string{
		"--enable", "cli_daybreak",
		"-c", "daybreak=false",
		"-c", "daybreak=true",
		"exec", "--cyber-access-program", "daybreak_blue", "review",
	}
	_, run, err := parseRunArguments(tail)
	if err != nil || !reflect.DeepEqual(run, tail) {
		t.Fatalf("run changed explicit native feature order: %#v, %v", run, err)
	}
	options, err := parseSuperArguments(append([]string{"--no-presidio", "--no-sub-agent"}, tail...))
	if err != nil {
		t.Fatal(err)
	}
	// Super adds its established apps-off override, but must not reorder,
	// invent or discard explicit native Daybreak option/config assignments.
	if len(options.codexArgs) != len(tail)+2 || !reflect.DeepEqual(options.codexArgs[2:], tail) {
		t.Fatalf("super changed explicit Daybreak precedence: %#v", options.codexArgs)
	}
}

// Prodex intentionally does not validate Codex's enum. An invalid spelling
// must reach upstream unchanged and be rejected by Codex 0.161, not normalized
// by Godex to a different, potentially privileged program.
func TestProdex04360CyberProgramValidationRemainsCodexOwned(t *testing.T) {
	tail := []string{"exec", "--cyber-access-program", "daybreak-blue", "review", "--uncommitted"}
	_, run, err := parseRunArguments(tail)
	if err != nil || !reflect.DeepEqual(run, tail) {
		t.Fatalf("Godex validated or rewrote Codex program: %#v err=%v", run, err)
	}
	super, err := parseSuperArguments(append([]string{"--no-presidio", "--no-sub-agent"}, tail...))
	if err != nil {
		t.Fatal(err)
	}
	if len(super.codexArgs) != len(tail)+2 || !reflect.DeepEqual(super.codexArgs[2:], tail) {
		t.Fatalf("Super rewrote invalid native program before upstream validation: %#v", super.codexArgs)
	}
}

// The tagged 0.436.0 CLI tests preserve the inline Cyber option for exec fork.
// The native session resolver must skip separated program values while keeping
// fork's selected UUID (the same option-arity contract as exec resume).
func TestProdex04360CyberProgramForkRetainsThreadIdentity(t *testing.T) {
	for _, program := range []string{"standard", "daybreak_blue", "daybreak_red"} {
		for _, args := range [][]string{
			{"exec", "--cyber-access-program", program, "fork", prodex04360Session, "continue"},
			{"exec", "fork", "--cyber-access-program", program, prodex04360Session, "continue"},
			{"exec", "fork", prodex04360Session, "--cyber-access-program", program, "continue"},
			{"exec", "fork", prodex04360Session, "--cyber-access-program=" + program, "continue"},
		} {
			index, actual := sessionArgument(args)
			want := slices.Index(args, prodex04360Session)
			if index != want || !reflect.DeepEqual(actual, args) {
				t.Fatalf("native fork dropped or misselected Cyber args: original=%#v index=%d want=%d got=%#v", args, index, want, actual)
			}
		}
	}
}

func TestProdex04360SuperWithoutExplicitProgramDoesNotInventDefaults(t *testing.T) {
	opts, err := parseSuperArguments([]string{"--no-presidio", "--no-sub-agent", "exec", "review"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range opts.codexArgs {
		if value == "--cyber-access-program" || value == "--enable" ||
			value == "cli_daybreak" || value == "daybreak=true" ||
			strings.HasPrefix(value, "model=") || strings.HasPrefix(value, "model_reasoning_effort=") {
			t.Fatalf("0.436.0 introduced unintended model/program/Daybreak default: %#v", opts.codexArgs)
		}
	}
}
