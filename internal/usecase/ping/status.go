package ping

import pingmodel "github.com/christiandoxa/godex/internal/model/ping"

func statusDetail(status pingmodel.Status) string {
	switch status {
	case pingmodel.Pass:
		return "valid model response received"
	case pingmodel.AuthFailed:
		return "OpenAI authentication failed"
	case pingmodel.DNSFailed:
		return "OpenAI hostname resolution failed"
	case pingmodel.TLSFailed:
		return "OpenAI TLS connection failed"
	case pingmodel.Timeout:
		return "OpenAI diagnostic timed out"
	case pingmodel.RateLimited:
		return "OpenAI temporarily rate limited the profile"
	case pingmodel.QuotaExhausted:
		return "OpenAI reported quota exhaustion"
	case pingmodel.UpstreamOverloaded:
		return "OpenAI upstream is temporarily unavailable"
	case pingmodel.ModelUnavailable:
		return "selected OpenAI model is unavailable"
	case pingmodel.ProtocolFailed:
		return "Codex did not complete a structured turn"
	case pingmodel.TurnFailed:
		return "Codex turn failed"
	case pingmodel.ProcessFailed:
		return "Codex diagnostic process failed"
	case pingmodel.SpawnFailed:
		return "OpenAI application ping could not start"
	case pingmodel.Cancelled:
		return "OpenAI ping was cancelled"
	case pingmodel.UnexpectedResponse:
		return "completed turn did not return a model response"
	default:
		return "OpenAI application ping failed"
	}
}

func humanStatus(status pingmodel.Status) string {
	switch status {
	case pingmodel.Pass:
		return "OK"
	case pingmodel.AuthFailed:
		return "AUTH_FAILED"
	case pingmodel.DNSFailed:
		return "DNS_FAILED"
	case pingmodel.TLSFailed:
		return "TLS_FAILED"
	case pingmodel.Timeout:
		return "TIMEOUT"
	case pingmodel.RateLimited:
		return "RATE_LIMITED"
	case pingmodel.QuotaExhausted:
		return "EXHAUSTED"
	case pingmodel.UpstreamOverloaded:
		return "UPSTREAM_OVERLOADED"
	case pingmodel.ModelUnavailable:
		return "MODEL_UNAVAILABLE"
	case pingmodel.ProtocolFailed:
		return "PROTOCOL_FAILED"
	case pingmodel.TurnFailed:
		return "TURN_FAILED"
	case pingmodel.ProcessFailed:
		return "PROCESS_FAILED"
	case pingmodel.SpawnFailed:
		return "SPAWN_FAILED"
	case pingmodel.Cancelled:
		return "CANCELLED"
	case pingmodel.UnexpectedResponse:
		return "UNEXPECTED_RESPONSE"
	default:
		return "FAILED"
	}
}

func temporary(status pingmodel.Status) bool {
	switch status {
	case pingmodel.DNSFailed, pingmodel.TLSFailed, pingmodel.Timeout, pingmodel.RateLimited, pingmodel.UpstreamOverloaded:
		return true
	default:
		return false
	}
}

func credentialValidation(status pingmodel.Status) string {
	switch status {
	case pingmodel.Pass:
		return "valid"
	case pingmodel.AuthFailed:
		return "failed"
	default:
		return "unknown"
	}
}
