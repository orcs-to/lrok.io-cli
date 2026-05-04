package protocol

const Version = 1

type RegisterRequest struct {
	Version   int    `json:"version"`
	AuthToken string `json:"auth_token,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

type RegisterResponse struct {
	OK        bool   `json:"ok"`
	Subdomain string `json:"subdomain,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
	Error     string `json:"error,omitempty"`
}
