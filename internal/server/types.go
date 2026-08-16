package server

import "time"

type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type OIDCSettings struct {
	Enabled           bool   `json:"enabled"`
	IssuerURL         string `json:"issuer_url"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret,omitempty"`
	ClearClientSecret bool   `json:"clear_client_secret,omitempty"`
	DisplayName       string `json:"display_name"`
	AutoProvision     bool   `json:"auto_provision"`
	DefaultRole       string `json:"default_role"`
}

type AISettings struct {
	Enabled               bool   `json:"enabled"`
	Provider              string `json:"provider"`
	BaseURL               string `json:"base_url"`
	APIKey                string `json:"api_key,omitempty"`
	ClearAPIKey           bool   `json:"clear_api_key,omitempty"`
	Model                 string `json:"model"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	SystemPrompt          string `json:"system_prompt"`
}

type ApprovalSettings struct {
	Enabled       bool     `json:"enabled"`
	ResourceTypes []string `json:"resource_types"`
	ReviewerRole  string   `json:"reviewer_role"`
}

type KeyPolicySettings struct {
	RotationDays      int      `json:"rotation_days"`
	AllowUserRotation bool     `json:"allow_user_rotation"`
	DefaultScopes     []string `json:"default_scopes"`
}
