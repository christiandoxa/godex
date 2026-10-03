package codex

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSessionAttachmentPathSuffixMatchesProdexComponents(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{filepath.Join(root, "overlay", "attachments", "id-1", "pasted-text-1.txt"), filepath.Join("id-1", "pasted-text-1.txt"), true},
		{filepath.Join(root, "overlay", "attachments", "id-2", "image-1.png"), filepath.Join("id-2", "image-1.png"), true},
		{filepath.Join(root, "overlay", "attachments", "id-3", "goal-objective.md"), filepath.Join("id-3", "goal-objective.md"), true},
		{filepath.Join(root, "overlay", "attachments", "id-4", "other.txt"), "", false},
		{filepath.Join(root, "overlay", "attachments", "id-5", "sub", "image-1.png"), "", false},
		{filepath.Join(root, "overlay", "attachments", "..", "image-1.png"), "", false},
		{filepath.Join(root, "overlay", "attachments", "id-6", ".."), "", false},
	}
	for _, test := range cases {
		got, ok := sessionAttachmentPathSuffix(test.path)
		if ok != test.ok || got != test.want {
			t.Errorf("suffix(%q) = %q,%t want %q,%t", test.path, got, ok, test.want, test.ok)
		}
	}
}

func TestSessionAttachmentsAreStableMatchesProdexRoots(t *testing.T) {
	home := t.TempDir()
	stableImage := filepath.Join(home, "image_attachments", "codex-clipboard-a.png")
	stablePaste := filepath.Join(home, "attachments", "id-a", "pasted-text-1.txt")
	if !sessionAttachmentsAreStable(home,
		"<image path=\""+stableImage+"\">\n"+
			"{\"local_images\":[\""+stableImage+"\"]}\n"+
			"{\"path\":\""+stablePaste+"\"}") {
		t.Fatal("stable shared-root attachment paths were reported unstable")
	}

	oldImage := filepath.Join(t.TempDir(), "codex-clipboard-old.png")
	if sessionAttachmentsAreStable(home, "<image path=\""+oldImage+"\">") {
		t.Fatal("ephemeral image tag path was reported stable")
	}
	if sessionAttachmentsAreStable(home, "{\"local_images\":[\""+oldImage+"\"]}") {
		t.Fatal("ephemeral inline clipboard path was reported stable")
	}
	oldPaste := filepath.Join(t.TempDir(), "attachments", "id-a", "pasted-text-1.txt")
	if sessionAttachmentsAreStable(home, "{\"path\":\""+oldPaste+"\"}") {
		t.Fatal("ephemeral attachment path was reported stable")
	}

	absoluteOther := filepath.Join(t.TempDir(), "attachments", "id-a", "other.txt")
	if !sessionAttachmentsAreStable(home, "{\"path\":\""+absoluteOther+"\"}") {
		t.Fatal("non-persistable attachment filename incorrectly made session unstable")
	}
}

func TestSessionPersistedAttachmentPathsDeduplicatesInProdexPassOrder(t *testing.T) {
	root := t.TempDir()
	image := filepath.Join(root, "codex-clipboard-a.png")
	attachment := filepath.Join(root, "attachments", "id-a", "image-1.png")
	contents := "<image path=\"" + image + "\">\n" +
		"{\"local_images\":[\"" + image + "\"]}\n" +
		"{\"path\":\"" + attachment + "\",\"again\":\"" + attachment + "\"}"
	got := sessionPersistedAttachmentPaths(contents)
	want := []string{image, attachment}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted paths = %#v, want %#v", got, want)
	}
}

func TestSessionAttachmentSuffixDoesNotNormalizeParentComponents(t *testing.T) {
	sep := string(filepath.Separator)
	path := sep + "tmp" + sep + "attachments" + sep + "id" + sep + ".." + sep + "image-1.png"
	if got, ok := sessionAttachmentPathSuffix(path); ok {
		t.Fatalf("parent-containing suffix accepted as %q", got)
	}
}

func TestSessionWindowsPathHelpersMatchProdexEscaping(t *testing.T) {
	raw := `C:\\Users\\runner\\attachments\\id-a\\image-1.png`
	if got := sessionPathEscapeWidth(raw); got != 2 {
		t.Fatalf("escape width = %d, want 2", got)
	}
	decoded := decodeSessionPathEscaped(raw, 2)
	want := `C:\Users\runner\attachments\id-a\image-1.png`
	if decoded != want {
		t.Fatalf("decoded path = %q, want %q", decoded, want)
	}
	got := sessionPathComponentsMode(decoded, true)
	wantParts := []string{"C:", "Users", "runner", "attachments", "id-a", "image-1.png"}
	if !reflect.DeepEqual(got, wantParts) {
		t.Fatalf("Windows components = %#v, want %#v", got, wantParts)
	}

	mixed := `C:\\Users\\\\runner`
	if got := decodeSessionPathEscaped(mixed, sessionPathEscapeWidth(mixed)); got != `C:\Users\\runner` {
		t.Fatalf("mixed escape decode = %q", got)
	}
}
