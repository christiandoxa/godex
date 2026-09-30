package account

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type profileTransaction struct {
	Version     int       `json:"version"`
	Kind        string    `json:"kind"`
	AccountID   string    `json:"account_id"`
	Backup      string    `json:"backup"`
	HadExisting bool      `json:"had_existing"`
	Next        stateFile `json:"next"`
}

func (store *FileStore) beginTransaction(kind, id, backup string, next stateFile) (profileTransaction, error) {
	original := store.accountDir(id)
	if kind == "auth" {
		original = filepath.Join(store.CodexHome(id), "auth.json")
	}
	_, err := os.Lstat(original)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return profileTransaction{}, err
	}
	transaction := profileTransaction{Version: 1, Kind: kind, AccountID: id, Backup: backup, HadExisting: err == nil, Next: next}
	next.Version = stateVersion
	transaction.Next = next
	if err := store.validateTransaction(transaction); err != nil {
		return transaction, err
	}
	content, err := json.Marshal(transaction)
	if err != nil {
		return transaction, err
	}
	file, err := os.CreateTemp(store.root, ".transaction-*")
	if err != nil {
		return transaction, err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return transaction, err
	}
	if err := fileutil.Replace(file.Name(), store.journalPath()); err != nil {
		return transaction, err
	}
	return transaction, fileutil.SyncDirectory(store.root)
}

func (store *FileStore) journalPath() string {
	return filepath.Join(store.root, "profile-transaction.json")
}

func (store *FileStore) validateTransaction(tx profileTransaction) error {
	if tx.Version != 1 {
		return errors.New("unsupported profile transaction version")
	}
	if err := validateState(tx.Next); err != nil {
		return err
	}
	original := store.accountDir(tx.AccountID)
	switch tx.Kind {
	case "profile", "remove":
	case "auth":
		original = filepath.Join(store.CodexHome(tx.AccountID), "auth.json")
	default:
		return errors.New("invalid profile transaction kind")
	}
	// Only validated IDs and sibling transaction paths may reach recovery mutations.
	if _, err := hex.DecodeString(tx.AccountID); err != nil || len(tx.AccountID) != 32 {
		return errors.New("invalid profile transaction account")
	}
	if filepath.Dir(tx.Backup) != filepath.Dir(original) || !strings.HasPrefix(filepath.Base(tx.Backup), filepath.Base(original)+".") {
		return errors.New("invalid profile transaction backup")
	}
	return nil
}

func (store *FileStore) recoverTransaction() error {
	info, err := os.Lstat(store.journalPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return errors.New("invalid profile transaction journal")
	}
	file, err := os.Open(store.journalPath())
	if err != nil {
		return err
	}
	content, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var tx profileTransaction
	if err := decoder.Decode(&tx); err != nil {
		return errors.New("decode profile transaction journal")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return errors.New("decode trailing profile transaction journal")
	}
	if err := store.validateTransaction(tx); err != nil {
		return err
	}
	state, err := store.readState()
	if err != nil {
		return err
	}
	if reflect.DeepEqual(state, tx.Next) {
		if err := os.RemoveAll(tx.Backup); err != nil {
			return err
		}
	} else if err := store.rollbackTransaction(tx); err != nil {
		return fmt.Errorf("recover profile transaction: %w", err)
	}
	if err := os.Remove(store.journalPath()); err != nil {
		return err
	}
	return fileutil.SyncDirectory(store.root)
}

func (store *FileStore) rollbackTransaction(tx profileTransaction) error {
	original := store.accountDir(tx.AccountID)
	if tx.Kind == "auth" {
		original = filepath.Join(store.CodexHome(tx.AccountID), "auth.json")
	}
	backup, err := os.Lstat(tx.Backup)
	if errors.Is(err, os.ErrNotExist) {
		if !tx.HadExisting && tx.Kind != "remove" {
			return os.RemoveAll(original)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if backup.Mode()&os.ModeSymlink != 0 {
		return errors.New("transaction backup must not be a symbolic link")
	}
	if tx.Kind == "auth" {
		return fileutil.Replace(tx.Backup, original)
	}
	if tx.Kind == "profile" {
		if err := os.RemoveAll(original); err != nil {
			return err
		}
	}
	if err := os.Rename(tx.Backup, original); err != nil {
		return err
	}
	return fileutil.SyncDirectory(store.accountsDir())
}

func (store *FileStore) readSnapshot(ctx context.Context) (state stateFile, err error) {
	err = store.withLock(ctx, func() error { var readErr error; state, readErr = store.readState(); return readErr })
	return state, err
}
