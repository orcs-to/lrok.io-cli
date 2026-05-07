// Package env detects the lrok environment (production / staging) once
// at process start and exposes the resolved API base, tunnel host, and
// config directory.
//
// Detection order (first match wins):
//   1. LROK_ENV env var: "staging" → staging defaults; anything else → prod
//   2. argv[0] basename: contains "staging" → staging defaults
//   3. otherwise: production
//
// Per-field env-var overrides (LROK_API_URL, LROK_TUNNEL_HOST, etc.)
// are applied AFTER detection so a staging-named binary can be pointed
// at any backend for ad-hoc development.
//
// The CLI ships as a single Go binary; the same release artifact serves
// both environments. The staging-install.sh / staging-install.ps1
// scripts install it under the name `staging-lrok` so detection picks
// up the right defaults without the user setting anything.

package env

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Env is the resolved environment configuration. Returned by Resolve()
// and stable for the process lifetime.
type Env struct {
	// Name is "production" or "staging" — surfaced in --version and in
	// telemetry context strings so we can distinguish staging traffic.
	Name string
	// APIBase is the dashboard API root (no trailing slash).
	APIBase string
	// TunnelHost is "host:port" — the yamux control plane.
	TunnelHost string
	// WebBase is the marketing/web origin used for `lrok login` browser
	// flow + self-update sources display.
	WebBase string
	// ConfigDirName is the per-environment subdirectory under the user's
	// HOME (or %USERPROFILE%) — config and tokens live separately so
	// the same machine can hold both prod and staging credentials.
	ConfigDirName string
	// TelemetryURL is where /track/error lifecycle beacons go. Empty
	// disables telemetry network entirely.
	TelemetryURL string
}

const (
	// Production defaults.
	prodAPIBase     = "https://api.lrok.io"
	prodTunnelHost  = "tunnel.lrok.io:7000"
	prodWebBase     = "https://lrok.io"
	prodConfigDir   = ".lrok"
	prodTelemetry   = "https://api.lrok.io/api/v1/track/error"

	// Staging defaults.
	// Tunnel host reuses tunnel.lrok.io because the Traefik wildcard cert
	// (*.lrok.io via DNS-01) covers it, and Traefik issues no second-level
	// wildcard for *.staging.lrok.io. Port 7001 is the staging differentiator
	// (prod is :7000) — staging's backend listens on :7001 inside the swarm.
	stagingAPIBase     = "https://api.staging.lrok.io"
	stagingTunnelHost  = "tunnel.lrok.io:7001"
	stagingWebBase     = "https://staging.lrok.io"
	stagingConfigDir   = ".lrok-staging"
	stagingTelemetry   = "https://api.staging.lrok.io/api/v1/track/error"
)

var (
	once sync.Once
	out  Env
)

// Resolve returns the environment for this process. Idempotent + cached;
// safe for concurrent calls.
func Resolve() Env {
	once.Do(func() {
		out = detect()
	})
	return out
}

func detect() Env {
	// 1. LROK_ENV env var wins over binary name. Useful for pointing a
	//    plain `lrok` invocation at staging during dev.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LROK_ENV"))) {
	case "staging", "stg", "dev":
		return applyOverrides(stagingDefaults())
	case "production", "prod", "":
		// fall through to next detection step — empty means "not set",
		// not "explicitly production"
	default:
		// Unknown LROK_ENV value: fall through to next step rather than
		// erroring; the override env vars below can still steer it.
	}

	// 2. argv[0] basename — staging-install.sh installs as `staging-lrok`.
	exe := strings.ToLower(filepath.Base(os.Args[0]))
	exe = strings.TrimSuffix(exe, ".exe")
	if strings.Contains(exe, "staging") {
		return applyOverrides(stagingDefaults())
	}

	// 3. Default = production.
	return applyOverrides(prodDefaults())
}

func prodDefaults() Env {
	return Env{
		Name:          "production",
		APIBase:       prodAPIBase,
		TunnelHost:    prodTunnelHost,
		WebBase:       prodWebBase,
		ConfigDirName: prodConfigDir,
		TelemetryURL:  prodTelemetry,
	}
}

func stagingDefaults() Env {
	return Env{
		Name:          "staging",
		APIBase:       stagingAPIBase,
		TunnelHost:    stagingTunnelHost,
		WebBase:       stagingWebBase,
		ConfigDirName: stagingConfigDir,
		TelemetryURL:  stagingTelemetry,
	}
}

// applyOverrides honors per-field env vars. Order matters: later fields
// can be overridden without touching earlier ones, and an empty string
// is treated as "no override" (we never blank out a default by accident).
func applyOverrides(e Env) Env {
	if v := strings.TrimSpace(os.Getenv("LROK_API_URL")); v != "" {
		e.APIBase = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("LROK_TUNNEL_HOST")); v != "" {
		e.TunnelHost = v
	}
	if v := strings.TrimSpace(os.Getenv("LROK_WEB_URL")); v != "" {
		e.WebBase = strings.TrimRight(v, "/")
	}
	return e
}
