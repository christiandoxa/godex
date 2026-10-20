package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func readCompactLineageOwner04383(p product) (string, error) {
	return readCompactBindingOwner04383(p, "turn", "__compact_turn_state__:compact-turn")
}

func readCompactSessionOwner04383(p product) (string, error) {
	return readCompactBindingOwner04383(p, "session", "__compact_session__:compact-produced-session")
}

func readCompactDirectSessionOwner04383(p product) (string, error) {
	return readCompactBindingOwner04383(p, "session", "compact-produced-session")
}

func readCompactBindingOwner04383(p product, kind, alias string) (string, error) {
	if kind != "turn" && kind != "session" {
		return "", errors.New("invalid synthetic Compact binding kind")
	}
	if p.name == "prodex" {
		raw, err := os.ReadFile(filepath.Join(p.state, "runtime-continuations.json"))
		if err != nil {
			return "", err
		}
		type binding struct {
			Owner string `json:"profile_name"`
		}
		var state struct {
			Value struct {
				Turns    map[string]binding `json:"turn_state_bindings"`
				Sessions map[string]binding `json:"session_id_bindings"`
			} `json:"value"`
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			return "", err
		}
		if kind == "turn" {
			return state.Value.Turns[alias].Owner, nil
		}
		return state.Value.Sessions[alias].Owner, nil
	}
	raw, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return "", err
	}
	var accounts struct{ Accounts []struct{ ID, Name string } }
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return "", err
	}
	names := make(map[string]string, len(accounts.Accounts))
	for _, account := range accounts.Accounts {
		names[account.ID] = account.Name
	}
	// Verified session observations with different owners produce the
	// official hard-binding conflict sentinel, not an unknown account.
	names[routingentity.ConflictAccountID] = routingentity.ConflictAccountID
	raw, err = os.ReadFile(filepath.Join(p.state, "routing.json"))
	if err != nil {
		return "", err
	}
	var bindings struct {
		Bindings []struct {
			Key   string
			Owner string `json:"account_id"`
		}
	}
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(kind + ":" + alias))
	for _, binding := range bindings.Bindings {
		if binding.Key == hex.EncodeToString(key[:]) {
			owner, valid := names[binding.Owner]
			if !valid {
				return "", errors.New("unregistered Compact lineage owner")
			}
			return owner, nil
		}
	}
	return "", nil
}
