package protocol

const Version = 1

type RegisterRequest struct {
	Version   int    `json:"version"`
	AuthToken string `json:"auth_token,omitempty"`
	Hint      string `json:"hint,omitempty"`
	// BasicAuth optionally gates the public tunnel URL with HTTP Basic Auth.
	// Format is the literal "user:pass" string; empty means no gating.
	BasicAuth string `json:"basic_auth,omitempty"`
}

type RegisterResponse struct {
	OK        bool   `json:"ok"`
	Subdomain string `json:"subdomain,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
	Error     string `json:"error,omitempty"`
}
