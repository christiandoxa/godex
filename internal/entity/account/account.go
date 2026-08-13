package account

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	accountIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	accountNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)
)

const maxAccountNameLength = 48

type Account struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Email            string    `json:"email,omitempty"`
	ChatGPTAccountID string    `json:"chatgpt_account_id,omitempty"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	LastUsedAt       time.Time `json:"last_used_at"`
}

type Identity struct {
	Email            string
	ChatGPTAccountID string
}

func NewAccount(identity Identity, requestedName string, now time.Time) (Account, error) {
	identity.Email = strings.TrimSpace(strings.ToLower(identity.Email))
	identity.ChatGPTAccountID = strings.TrimSpace(identity.ChatGPTAccountID)
	if identity.Email == "" && identity.ChatGPTAccountID == "" {
		return Account{}, errors.New("ChatGPT login did not expose an account identity")
	}

	name := requestedName
	if name == "" {
		name = defaultName(identity.Email)
	}
	name, err := NormalizeAccountName(name)
	if err != nil {
		return Account{}, err
	}

	return Account{
		ID:               StableAccountID(identity),
		Name:             name,
		Email:            identity.Email,
		ChatGPTAccountID: identity.ChatGPTAccountID,
		Enabled:          true,
		CreatedAt:        now.UTC(),
		UpdatedAt:        now.UTC(),
	}, nil
}

func StableAccountID(identity Identity) string {
	key := strings.TrimSpace(identity.ChatGPTAccountID)
	if key != "" {
		key = "chatgpt:" + key
	} else {
		key = "email:" + strings.ToLower(strings.TrimSpace(identity.Email))
	}

	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:16])
}

func ValidateAccount(account Account) error {
	if !accountIDPattern.MatchString(account.ID) {
		return fmt.Errorf("invalid account ID %q", account.ID)
	}
	normalizedName, err := NormalizeAccountName(account.Name)
	if err != nil || normalizedName != account.Name {
		return fmt.Errorf("invalid account name %q", account.Name)
	}
	if strings.TrimSpace(account.Email) == "" && strings.TrimSpace(account.ChatGPTAccountID) == "" {
		return errors.New("account has no ChatGPT identity")
	}
	return nil
}

func NormalizeAccountName(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "._-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > maxAccountNameLength {
		value = strings.Trim(value[:maxAccountNameLength], "._-")
	}
	if !accountNamePattern.MatchString(value) {
		return "", fmt.Errorf("invalid account name %q; use letters, digits, dot, underscore, or dash", value)
	}
	return value, nil
}

func (account Account) Matches(selector string) bool {
	selector = strings.TrimSpace(selector)
	return selector != "" && (strings.EqualFold(account.ID, selector) ||
		strings.EqualFold(account.Name, selector) ||
		strings.EqualFold(strings.TrimSpace(account.Email), selector))
}

func (account Account) SameIdentity(identity Identity) bool {
	accountID := strings.TrimSpace(account.ChatGPTAccountID)
	identityID := strings.TrimSpace(identity.ChatGPTAccountID)
	if accountID != "" && identityID != "" {
		return accountID == identityID
	}
	accountEmail := strings.TrimSpace(account.Email)
	identityEmail := strings.TrimSpace(identity.Email)
	return accountEmail != "" && identityEmail != "" && strings.EqualFold(accountEmail, identityEmail)
}

func defaultName(email string) string {
	local := email
	if before, _, found := strings.Cut(email, "@"); found {
		local = before
	}
	if local == "" {
		return "chatgpt"
	}
	return local
}
