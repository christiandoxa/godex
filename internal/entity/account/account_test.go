package account

import (
	"testing"
	"time"
)

func TestStableAccountIDPrefersChatGPTAccountID(t *testing.T) {
	first := StableAccountID(Identity{Email: "one@example.com", ChatGPTAccountID: "acct-1"})
	second := StableAccountID(Identity{Email: "changed@example.com", ChatGPTAccountID: "acct-1"})
	if first != second {
		t.Fatalf("stable IDs differ: %q != %q", first, second)
	}
	if len(first) != 32 {
		t.Fatalf("stable ID length = %d", len(first))
	}
}

func TestNewAccountNormalizesDefaultName(t *testing.T) {
	account, err := NewAccount(Identity{Email: "Jane+Work@Example.com"}, "", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "jane-work" {
		t.Fatalf("name = %q", account.Name)
	}
	if account.Email != "jane+work@example.com" {
		t.Fatalf("email = %q", account.Email)
	}
}

func TestNormalizeAccountNameRejectsEmpty(t *testing.T) {
	if _, err := NormalizeAccountName("---"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestValidateAccountRejectsUnsafeID(t *testing.T) {
	account, err := NewAccount(Identity{Email: "person@example.com"}, "person", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	account.ID = "../../outside"
	if err := ValidateAccount(account); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSameIdentityFallsBackToEmailWhenOneAccountIDIsMissing(t *testing.T) {
	account, err := NewAccount(Identity{
		Email:            "person@example.com",
		ChatGPTAccountID: "account-1",
	}, "person", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !account.SameIdentity(Identity{Email: " PERSON@example.com "}) {
		t.Fatal("expected email fallback to match")
	}
}

func TestSameIdentityRejectsDifferentAccountIDsWithSameEmail(t *testing.T) {
	account, err := NewAccount(Identity{
		Email:            "person@example.com",
		ChatGPTAccountID: "account-1",
	}, "person", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if account.SameIdentity(Identity{
		Email:            "person@example.com",
		ChatGPTAccountID: "account-2",
	}) {
		t.Fatal("different ChatGPT accounts must not deduplicate")
	}
}

func TestMatchesUsesTrimmedExactFields(t *testing.T) {
	account, err := NewAccount(Identity{Email: "person@example.com"}, "work", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !account.Matches(" PERSON@EXAMPLE.COM ") || account.Matches("per") {
		t.Fatal("selector matching is not exact")
	}
}

func TestNormalizeAccountNameLimitsNameLength(t *testing.T) {
	name, err := NormalizeAccountName("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(name) != maxAccountNameLength {
		t.Fatalf("name length = %d", len(name))
	}
}
