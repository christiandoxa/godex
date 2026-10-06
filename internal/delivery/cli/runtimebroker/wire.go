package runtimebroker

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	BootstrapVersion    = 1
	BootstrapMaxBytes   = 64 * 1024
	CapabilityVersion   = 1
	CapabilityMaxBytes  = 8 * 1024
	SecretMaxBytes      = 4 * 1024
	OpenAIMountPath     = "/backend-api/godex"
	AdminTokenHeader    = "X-Godex-Admin-Token"
	HealthPath          = "/__godex/runtime/health"
	MetricsPath         = "/__godex/runtime/metrics"
	ActivatePath        = "/__godex/runtime/activate"
	ReleaseAffinityPath = "/__godex/runtime/session-affinity/release"
	LogSnapshotPath     = "/__godex/runtime/log/snapshot"
	LogEventPath        = "/__godex/runtime/log/event"
)

type Secret struct {
	value []byte
}

func NewSecret(value string) (Secret, error) {
	if value == "" || len(value) > SecretMaxBytes {
		return Secret{}, errors.New("runtime broker secret is invalid")
	}
	return Secret{value: []byte(value)}, nil
}

func (secret Secret) String() string { return "<redacted>" }

func (secret Secret) Expose() string { return string(secret.value) }

func (secret Secret) Matches(candidate string) bool {
	expected := secret.value
	actual := []byte(candidate)
	difference := len(expected) ^ len(actual)
	for index, value := range expected {
		var other byte
		if index < len(actual) {
			other = actual[index]
		}
		difference |= int(value ^ other)
	}
	return subtle.ConstantTimeEq(int32(difference), 0) == 1
}

type Bootstrap struct {
	CurrentProfile           string
	UpstreamBaseURL          string
	IncludeCodeReview        bool
	UpstreamNoProxy          bool
	SmartContextEnabled      bool
	ModelContextWindowTokens *uint64
	BrokerKey                string
	InstanceID               string
	AdminToken               Secret
	ListenAddr               string
}

type bootstrapWire struct {
	Version                  uint16  `json:"version"`
	CurrentProfile           string  `json:"current_profile"`
	UpstreamBaseURL          string  `json:"upstream_base_url"`
	IncludeCodeReview        bool    `json:"include_code_review"`
	UpstreamNoProxy          bool    `json:"upstream_no_proxy"`
	SmartContextEnabled      bool    `json:"smart_context_enabled"`
	ModelContextWindowTokens *uint64 `json:"model_context_window_tokens"`
	BrokerKey                string  `json:"broker_key"`
	InstanceID               string  `json:"instance_id"`
	AdminToken               string  `json:"admin_token"`
	ListenAddr               *string `json:"listen_addr"`
}

type Capability struct {
	InstanceID string
	AdminToken Secret
}

type capabilityWire struct {
	Version    uint16 `json:"version"`
	InstanceID string `json:"instance_id"`
	AdminToken string `json:"admin_token"`
}

type Registry struct {
	PID                  uint32  `json:"pid"`
	ProcessBirthIdentity *string `json:"process_birth_identity,omitempty"`
	ListenAddr           string  `json:"listen_addr"`
	StartedAt            int64   `json:"started_at"`
	UpstreamBaseURL      string  `json:"upstream_base_url"`
	IncludeCodeReview    bool    `json:"include_code_review"`
	UpstreamNoProxy      bool    `json:"upstream_no_proxy"`
	SmartContextEnabled  bool    `json:"smart_context_enabled"`
	CurrentProfile       string  `json:"current_profile"`
	InstanceID           string  `json:"instance_id"`
	ProdexVersion        *string `json:"prodex_version,omitempty"`
	ExecutablePath       *string `json:"executable_path,omitempty"`
	ExecutableSHA256     *string `json:"executable_sha256,omitempty"`
	OpenAIMountPath      *string `json:"openai_mount_path,omitempty"`
	RealtimeWSAddr       *string `json:"realtime_ws_addr,omitempty"`
}

type Health struct {
	PID               uint32  `json:"pid"`
	StartedAt         int64   `json:"started_at"`
	CurrentProfile    string  `json:"current_profile"`
	IncludeCodeReview bool    `json:"include_code_review"`
	ActiveRequests    int     `json:"active_requests"`
	InstanceID        string  `json:"instance_id"`
	PersistenceRole   string  `json:"persistence_role"`
	ProdexVersion     *string `json:"prodex_version,omitempty"`
	ExecutablePath    *string `json:"executable_path,omitempty"`
	ExecutableSHA256  *string `json:"executable_sha256,omitempty"`
}

func ReadBootstrap(reader io.Reader) (Bootstrap, error) {
	payload, err := readBounded(reader, BootstrapMaxBytes, "runtime broker bootstrap")
	if err != nil {
		return Bootstrap{}, err
	}
	var wire bootstrapWire
	if err := decodeStrict(payload, &wire); err != nil {
		return Bootstrap{}, errors.New("runtime broker bootstrap is invalid")
	}
	if wire.Version != BootstrapVersion {
		return Bootstrap{}, errors.New("runtime broker bootstrap version is unsupported")
	}
	listen := ""
	if wire.ListenAddr != nil {
		listen = *wire.ListenAddr
	}
	if !validBootstrapFields(wire.CurrentProfile, wire.UpstreamBaseURL, wire.BrokerKey, wire.InstanceID, listen) {
		return Bootstrap{}, errors.New("runtime broker bootstrap is invalid")
	}
	secret, err := NewSecret(wire.AdminToken)
	if err != nil {
		return Bootstrap{}, errors.New("runtime broker bootstrap is invalid")
	}
	return Bootstrap{
		CurrentProfile: wire.CurrentProfile, UpstreamBaseURL: wire.UpstreamBaseURL,
		IncludeCodeReview: wire.IncludeCodeReview, UpstreamNoProxy: wire.UpstreamNoProxy,
		SmartContextEnabled:      wire.SmartContextEnabled,
		ModelContextWindowTokens: wire.ModelContextWindowTokens,
		BrokerKey:                wire.BrokerKey, InstanceID: wire.InstanceID, AdminToken: secret,
		ListenAddr: listen,
	}, nil
}

func EncodeCapability(instanceID string, secret Secret) ([]byte, error) {
	if !validID(instanceID) || len(secret.value) == 0 || len(secret.value) > SecretMaxBytes {
		return nil, errors.New("runtime broker capability is invalid")
	}
	payload, err := json.Marshal(capabilityWire{
		Version: CapabilityVersion, InstanceID: instanceID, AdminToken: secret.Expose(),
	})
	if err != nil || len(payload) > CapabilityMaxBytes {
		return nil, errors.New("runtime broker capability is invalid")
	}
	return payload, nil
}

func DecodeCapability(payload []byte) (Capability, error) {
	if len(payload) > CapabilityMaxBytes {
		return Capability{}, errors.New("runtime broker capability exceeds the size limit")
	}
	var wire capabilityWire
	if err := decodeStrict(payload, &wire); err != nil {
		return Capability{}, errors.New("runtime broker capability is invalid")
	}
	if wire.Version != CapabilityVersion {
		return Capability{}, errors.New("runtime broker capability version is unsupported")
	}
	if !validID(wire.InstanceID) {
		return Capability{}, errors.New("runtime broker capability is invalid")
	}
	secret, err := NewSecret(wire.AdminToken)
	if err != nil {
		return Capability{}, errors.New("runtime broker capability is invalid")
	}
	return Capability{InstanceID: wire.InstanceID, AdminToken: secret}, nil
}

func readBounded(reader io.Reader, limit int64, label string) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s I/O failed", label)
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("%s exceeds the size limit", label)
	}
	return payload, nil
}

func decodeStrict(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func validBootstrapFields(profile, upstream, brokerKey, instanceID, listen string) bool {
	return profile != "" && len(profile) <= 256 &&
		upstream != "" && len(upstream) <= 4096 &&
		validID(brokerKey) && validID(instanceID) &&
		(listen == "" || len(listen) <= 256 && listenAddrIsLoopback(listen))
}

func validID(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func listenAddrIsLoopback(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(port) == "" {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
