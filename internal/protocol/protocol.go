package protocol

const Version = 1

type RegisterRequest struct {
	Version   int    `json:"version"`
	AuthToken string `json:"auth_token,omitempty"`
	Hint      string `json:"hint,omitempty"`
	// Mode selects the tunnel kind. "" or "http" is HTTP (existing behavior),
	// "tcp" requests a raw TCP tunnel.
	Mode string `json:"mode,omitempty"`
	// BasicAuth optionally gates the public tunnel URL with HTTP Basic Auth.
	// Format is the literal "user:pass" string; empty means no gating. HTTP
	// mode only.
	BasicAuth string `json:"basic_auth,omitempty"`
}

// Tunnel modes for RegisterRequest.Mode.
const (
	ModeHTTP = "http"
	ModeTCP  = "tcp"
)

type RegisterResponse struct {
	OK        bool   `json:"ok"`
	Subdomain string `json:"subdomain,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
	// PublicAddr is the host:port to dial for tcp-mode tunnels (e.g.
	// "tcp.lrok.io:30042"). Empty for http-mode.
	PublicAddr string `json:"public_addr,omitempty"`
	Error      string `json:"error,omitempty"`
}
