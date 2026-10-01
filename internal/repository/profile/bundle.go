package profile

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/agl/gcmsiv"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	"golang.org/x/crypto/argon2"
)

const (
	invalidProfileExportEnvelope        = "invalid profile export envelope"
	profileExportFormat                 = "prodex_profile_export"
	profileExportCipher                 = "aes_256_gcm_siv"
	profileExportLegacyKDF              = "pbkdf2_sha256"
	profileExportArgon2KDF              = "argon2id"
	profileExportVersionV1       uint32 = 1
	profileExportVersionV2       uint32 = 2

	profileExportBundleMaxBytes              = 64 * 1024 * 1024
	profileExportPlaintextMaxBytes           = 32 * 1024 * 1024
	profileExportNestedJSONMaxBytes          = 2 * 1024 * 1024
	profileExportPasswordMaxBytes            = 4 * 1024
	profileExportMaxProfiles                 = 256
	profileExportMaxSecretsPerProfile        = 16
	profileExportMaxSecrets                  = profileExportMaxProfiles * profileExportMaxSecretsPerProfile
	profileExportSaltBytes                   = 16
	profileExportNonceBytes                  = 12
	profileExportKeyBytes                    = 32
	profileExportAuthTagBytes                = 16
	profileExportPBKDF2Iterations            = 100_000
	profileExportPBKDF2MinIterations         = 50_000
	profileExportPBKDF2MaxIterations         = 2_000_000
	profileExportArgon2Version        uint32 = 0x13
	profileExportArgon2Memory         uint32 = 64 * 1024
	profileExportArgon2Time           uint32 = 3
	profileExportArgon2Threads        uint8  = 1
)

type plainEnvelope struct {
	PayloadKind string                     `json:"payload_kind"`
	Format      string                     `json:"format"`
	Version     uint32                     `json:"version"`
	Payload     profilemodel.BundlePayload `json:"payload"`
}

type encryptedV1Envelope struct {
	PayloadKind      string `json:"payload_kind"`
	Format           string `json:"format"`
	Version          uint32 `json:"version"`
	Cipher           string `json:"cipher"`
	KDF              string `json:"kdf"`
	Iterations       uint32 `json:"iterations"`
	SaltBase64       string `json:"salt_base64"`
	NonceBase64      string `json:"nonce_base64"`
	CiphertextBase64 string `json:"ciphertext_base64"`
}

type argon2Parameters struct {
	Algorithm   string `json:"algorithm"`
	Version     uint32 `json:"version"`
	MemoryKiB   uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint32 `json:"parallelism"`
}

type encryptedV2Envelope struct {
	PayloadKind      string           `json:"payload_kind"`
	Format           string           `json:"format"`
	Version          uint32           `json:"version"`
	Cipher           string           `json:"cipher"`
	KDF              argon2Parameters `json:"kdf"`
	SaltBase64       string           `json:"salt_base64"`
	NonceBase64      string           `json:"nonce_base64"`
	CiphertextBase64 string           `json:"ciphertext_base64"`
}

type envelopeHeader struct {
	PayloadKind string `json:"payload_kind"`
	Format      string `json:"format"`
	Version     uint32 `json:"version"`
}

func EncodeProfileBundle(payload profilemodel.BundlePayload, password string) ([]byte, error) {
	if err := validateBundlePayload(payload); err != nil {
		return nil, err
	}
	if password == "" {
		content, err := json.MarshalIndent(plainEnvelope{
			PayloadKind: "plain", Format: profileExportFormat,
			Version: profileExportVersionV1, Payload: payload,
		}, "", "  ")
		return boundedBundle(content, err)
	}
	if len(password) > profileExportPasswordMaxBytes {
		return nil, fmt.Errorf("profile export password must contain 1..=%d bytes", profileExportPasswordMaxBytes)
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("failed to serialize profile export payload")
	}
	if len(plaintext) > profileExportPlaintextMaxBytes {
		return nil, fmt.Errorf("profile export payload exceeds safe size limit (%d bytes)", profileExportPlaintextMaxBytes)
	}
	salt := make([]byte, profileExportSaltBytes)
	nonce := make([]byte, profileExportNonceBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, errors.New("failed to generate export salt")
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.New("failed to generate export nonce")
	}
	key := argon2.IDKey([]byte(password), salt, profileExportArgon2Time, profileExportArgon2Memory, profileExportArgon2Threads, profileExportKeyBytes)
	defer clearBytes(key)
	cipher, err := gcmsiv.NewGCMSIV(key)
	if err != nil {
		return nil, errors.New("failed to initialize export cipher")
	}
	ciphertext := cipher.Seal(nil, nonce, plaintext, nil)
	envelope := encryptedV2Envelope{
		PayloadKind: "encrypted_v2", Format: profileExportFormat, Version: profileExportVersionV2,
		Cipher:     profileExportCipher,
		KDF:        argon2Parameters{Algorithm: profileExportArgon2KDF, Version: profileExportArgon2Version, MemoryKiB: profileExportArgon2Memory, Iterations: profileExportArgon2Time, Parallelism: uint32(profileExportArgon2Threads)},
		SaltBase64: base64.StdEncoding.EncodeToString(salt), NonceBase64: base64.StdEncoding.EncodeToString(nonce), CiphertextBase64: base64.StdEncoding.EncodeToString(ciphertext),
	}
	content, err := json.MarshalIndent(envelope, "", "  ")
	return boundedBundle(content, err)
}

func DecodeProfileBundle(content []byte, password string) (profilemodel.BundlePayload, bool, error) {
	if len(content) > profileExportBundleMaxBytes {
		return profilemodel.BundlePayload{}, false, fmt.Errorf("profile export bundle exceeds safe size limit (%d bytes)", profileExportBundleMaxBytes)
	}
	var header envelopeHeader
	if err := json.Unmarshal(content, &header); err != nil {
		return profilemodel.BundlePayload{}, false, errors.New(invalidProfileExportEnvelope)
	}
	if header.Format != profileExportFormat {
		return profilemodel.BundlePayload{}, false, errors.New("unsupported profile export format")
	}
	switch header.PayloadKind {
	case "plain":
		if header.Version != profileExportVersionV1 {
			return profilemodel.BundlePayload{}, false, errors.New(invalidProfileExportEnvelope)
		}
		var envelope plainEnvelope
		if err := json.Unmarshal(content, &envelope); err != nil {
			return profilemodel.BundlePayload{}, false, errors.New(invalidProfileExportEnvelope)
		}
		if err := validateBundlePayload(envelope.Payload); err != nil {
			return profilemodel.BundlePayload{}, false, err
		}
		return envelope.Payload, false, nil
	case "encrypted":
		payload, err := decodeEncryptedV1(content, password)
		return payload, true, err
	case "encrypted_v2":
		payload, err := decodeEncryptedV2(content, password)
		return payload, true, err
	default:
		return profilemodel.BundlePayload{}, false, errors.New(invalidProfileExportEnvelope)
	}
}

func decodeEncryptedV1(content []byte, password string) (profilemodel.BundlePayload, error) {
	var envelope encryptedV1Envelope
	if err := json.Unmarshal(content, &envelope); err != nil || envelope.Version != profileExportVersionV1 || envelope.Cipher != profileExportCipher || envelope.KDF != profileExportLegacyKDF || envelope.Iterations < profileExportPBKDF2MinIterations || envelope.Iterations > profileExportPBKDF2MaxIterations {
		return profilemodel.BundlePayload{}, errors.New(invalidProfileExportEnvelope)
	}
	salt, nonce, ciphertext, err := decodeCryptoFields(envelope.SaltBase64, envelope.NonceBase64, envelope.CiphertextBase64)
	if err != nil {
		return profilemodel.BundlePayload{}, err
	}
	if err := validatePassword(password); err != nil {
		return profilemodel.BundlePayload{}, err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, int(envelope.Iterations), profileExportKeyBytes)
	if err != nil {
		return profilemodel.BundlePayload{}, errors.New("failed to derive profile export key")
	}
	defer clearBytes(key)
	return decryptPayload(key, nonce, ciphertext)
}

func decodeEncryptedV2(content []byte, password string) (profilemodel.BundlePayload, error) {
	var envelope encryptedV2Envelope
	if err := json.Unmarshal(content, &envelope); err != nil || envelope.Version != profileExportVersionV2 || envelope.Cipher != profileExportCipher {
		return profilemodel.BundlePayload{}, errors.New(invalidProfileExportEnvelope)
	}
	parameters := envelope.KDF
	if parameters.Algorithm != profileExportArgon2KDF || parameters.Version != profileExportArgon2Version || parameters.MemoryKiB < 8*1024 || parameters.MemoryKiB > 128*1024 || parameters.Iterations < 1 || parameters.Iterations > 6 || parameters.Parallelism < 1 || parameters.Parallelism > 4 {
		return profilemodel.BundlePayload{}, errors.New(invalidProfileExportEnvelope)
	}
	salt, nonce, ciphertext, err := decodeCryptoFields(envelope.SaltBase64, envelope.NonceBase64, envelope.CiphertextBase64)
	if err != nil {
		return profilemodel.BundlePayload{}, err
	}
	if err := validatePassword(password); err != nil {
		return profilemodel.BundlePayload{}, err
	}
	key := argon2.IDKey([]byte(password), salt, parameters.Iterations, parameters.MemoryKiB, uint8(parameters.Parallelism), profileExportKeyBytes)
	defer clearBytes(key)
	return decryptPayload(key, nonce, ciphertext)
}

func decodeCryptoFields(saltText, nonceText, ciphertextText string) ([]byte, []byte, []byte, error) {
	salt, err := base64.StdEncoding.DecodeString(saltText)
	if err != nil || len(salt) != profileExportSaltBytes {
		return nil, nil, nil, errors.New(invalidProfileExportEnvelope)
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceText)
	if err != nil || len(nonce) != profileExportNonceBytes {
		return nil, nil, nil, errors.New(invalidProfileExportEnvelope)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextText)
	if err != nil || len(ciphertext) < profileExportAuthTagBytes || len(ciphertext) > profileExportPlaintextMaxBytes+profileExportAuthTagBytes {
		return nil, nil, nil, errors.New(invalidProfileExportEnvelope)
	}
	return salt, nonce, ciphertext, nil
}

func decryptPayload(key, nonce, ciphertext []byte) (profilemodel.BundlePayload, error) {
	cipher, err := gcmsiv.NewGCMSIV(key)
	if err != nil {
		return profilemodel.BundlePayload{}, errors.New("failed to initialize import cipher")
	}
	plaintext, err := cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return profilemodel.BundlePayload{}, errors.New("failed to decrypt profile export bundle")
	}
	if len(plaintext) > profileExportPlaintextMaxBytes {
		return profilemodel.BundlePayload{}, fmt.Errorf("decrypted profile export payload exceeds safe size limit (%d bytes)", profileExportPlaintextMaxBytes)
	}
	var payload profilemodel.BundlePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return profilemodel.BundlePayload{}, errors.New("failed to parse decrypted profile export payload")
	}
	if err := validateBundlePayload(payload); err != nil {
		return profilemodel.BundlePayload{}, err
	}
	return payload, nil
}

func validateBundlePayload(payload profilemodel.BundlePayload) error {
	if len(payload.Profiles) == 0 {
		return errors.New("profile export bundle does not contain any profiles")
	}
	if len(payload.Profiles) > profileExportMaxProfiles {
		return fmt.Errorf("profile export exceeds profile count limit (%d)", profileExportMaxProfiles)
	}
	seen := make(map[string]bool, len(payload.Profiles))
	secretCount := 0
	for _, profile := range payload.Profiles {
		if seen[profile.Name] {
			return fmt.Errorf("profile export bundle contains duplicate profile %q", profile.Name)
		}
		seen[profile.Name] = true
		if len(profile.AuthJSON) > profileExportNestedJSONMaxBytes {
			return fmt.Errorf("profile export nested secret exceeds size limit (%d bytes)", profileExportNestedJSONMaxBytes)
		}
		if len(profile.SecretFiles) > profileExportMaxSecretsPerProfile {
			return fmt.Errorf("profile export exceeds per-profile secret-file count limit (%d)", profileExportMaxSecretsPerProfile)
		}
		secretCount += len(profile.SecretFiles)
		for _, secret := range profile.SecretFiles {
			if len(secret.Text) > profileExportNestedJSONMaxBytes {
				return fmt.Errorf("profile export nested secret exceeds size limit (%d bytes)", profileExportNestedJSONMaxBytes)
			}
		}
	}
	if secretCount > profileExportMaxSecrets {
		return fmt.Errorf("profile export exceeds secret-file count limit (%d)", profileExportMaxSecrets)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) == 0 || len(password) > profileExportPasswordMaxBytes {
		return fmt.Errorf("profile export password must contain 1..=%d bytes", profileExportPasswordMaxBytes)
	}
	return nil
}

func boundedBundle(content []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, errors.New("failed to serialize profile export bundle")
	}
	if len(content) > profileExportBundleMaxBytes {
		return nil, fmt.Errorf("profile export bundle exceeds safe size limit (%d bytes)", profileExportBundleMaxBytes)
	}
	return content, nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func (store *Store) EncodeBundle(payload profilemodel.BundlePayload, password string) ([]byte, error) {
	return EncodeProfileBundle(payload, password)
}

func (store *Store) DecodeBundle(content []byte, password string) (profilemodel.BundlePayload, bool, error) {
	return DecodeProfileBundle(content, password)
}

func (store *Store) WriteBundle(path string, content []byte) error {
	return WriteProfileBundle(path, content)
}

func (store *Store) ReadBundle(path string) ([]byte, error) {
	return ReadProfileBundle(path)
}
