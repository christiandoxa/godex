package profile

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agl/gcmsiv"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const bundleTestPassword = "fixture-password"

func TestProfileBundlePlainAndEncryptedRoundTrip(t *testing.T) {
	payload := bundleFixture()
	plain, err := EncodeProfileBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	var plainHeader map[string]any
	if err := json.Unmarshal(plain, &plainHeader); err != nil {
		t.Fatal(err)
	}
	if plainHeader["payload_kind"] != "plain" || plainHeader["format"] != profileExportFormat {
		t.Fatalf("plain header = %#v", plainHeader)
	}
	decoded, encrypted, err := DecodeProfileBundle(plain, "")
	if err != nil || encrypted || decoded.Profiles[0].AuthJSON != payload.Profiles[0].AuthJSON {
		t.Fatalf("plain decoded = %#v, encrypted=%t, err=%v", decoded, encrypted, err)
	}

	protected, err := EncodeProfileBundle(payload, bundleTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	var protectedHeader struct {
		PayloadKind string           `json:"payload_kind"`
		Version     uint32           `json:"version"`
		KDF         argon2Parameters `json:"kdf"`
	}
	if err := json.Unmarshal(protected, &protectedHeader); err != nil {
		t.Fatal(err)
	}
	if protectedHeader.PayloadKind != "encrypted_v2" || protectedHeader.Version != 2 || protectedHeader.KDF.Algorithm != "argon2id" {
		t.Fatalf("encrypted header = %#v", protectedHeader)
	}
	decoded, encrypted, err = DecodeProfileBundle(protected, bundleTestPassword)
	if err != nil || !encrypted || decoded.Profiles[0].Name != "work" {
		t.Fatalf("encrypted decoded = %#v, encrypted=%t, err=%v", decoded, encrypted, err)
	}
	if _, _, err := DecodeProfileBundle(protected, "wrong-password"); err == nil {
		t.Fatal("wrong password unexpectedly decrypted bundle")
	}
}

func TestProfileBundleImportsLegacyPBKDF2Envelope(t *testing.T) {
	payload := bundleFixture()
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")
	nonce := []byte("0123456789ab")
	key, err := pbkdf2.Key(sha256.New, bundleTestPassword, salt, profileExportPBKDF2Iterations, profileExportKeyBytes)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := gcmsiv.NewGCMSIV(key)
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(encryptedV1Envelope{
		PayloadKind: "encrypted", Format: profileExportFormat, Version: 1,
		Cipher: profileExportCipher, KDF: profileExportLegacyKDF, Iterations: profileExportPBKDF2Iterations,
		SaltBase64: base64.StdEncoding.EncodeToString(salt), NonceBase64: base64.StdEncoding.EncodeToString(nonce),
		CiphertextBase64: base64.StdEncoding.EncodeToString(cipher.Seal(nil, nonce, plaintext, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, encrypted, err := DecodeProfileBundle(content, bundleTestPassword)
	if err != nil || !encrypted || decoded.Profiles[0].Name != "work" {
		t.Fatalf("legacy decoded = %#v, encrypted=%t, err=%v", decoded, encrypted, err)
	}
}

func TestProfileBundleRejectsUnsafeBoundsAndPrivateIO(t *testing.T) {
	payload := bundleFixture()
	payload.Profiles = make([]profilemodel.ExportedProfile, profileExportMaxProfiles+1)
	if _, err := EncodeProfileBundle(payload, ""); err == nil {
		t.Fatal("oversized profile collection accepted")
	}
	if _, err := EncodeProfileBundle(bundleFixture(), string(make([]byte, profileExportPasswordMaxBytes+1))); err == nil {
		t.Fatal("oversized password accepted")
	}

	root := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "profiles.json")
	content, err := EncodeProfileBundle(bundleFixture(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteProfileBundle(path, content); err != nil {
		t.Fatal(err)
	}
	read, err := ReadProfileBundle(path)
	if err != nil || string(read) != string(content) {
		t.Fatalf("read bundle = %v, equal=%t", err, string(read) == string(content))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("bundle mode = %o", info.Mode().Perm())
		}
	}
}

func bundleFixture() profilemodel.BundlePayload {
	active := "work"
	email := "person@example.test"
	return profilemodel.BundlePayload{
		ExportedAt: "2026-10-01T00:00:00Z", SourceProdexVersion: "0.434.2", ActiveProfile: &active,
		Profiles: []profilemodel.ExportedProfile{{
			Name: "work", Email: &email, SourceManaged: true,
			Provider:    profilemodel.ProviderSnapshot{Kind: "openai"},
			AuthJSON:    `{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-token"}}`,
			SecretFiles: []profilemodel.ExportedSecretFile{},
		}},
	}
}
