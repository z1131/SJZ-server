package dto

type QwenAuthStatusResponse struct {
	Provider      string                 `json:"provider"`
	Status        string                 `json:"status"`
	Connected     bool                   `json:"connected"`
	AuthMethod    string                 `json:"auth_method,omitempty"`
	AccountID     string                 `json:"account_id,omitempty"`
	Email         string                 `json:"email,omitempty"`
	Error         string                 `json:"error,omitempty"`
	PendingDevice *QwenPendingDeviceInfo `json:"pending_device,omitempty"`
}

type QwenPendingDeviceInfo struct {
	UserCode          string `json:"user_code"`
	VerifyURL         string `json:"verify_url"`
	VerifyURLComplete string `json:"verify_url_complete"`
	ExpiresIn         int    `json:"expires_in"`
}

type StartQwenAuthResponse struct {
	Status            string `json:"status"`
	UserCode          string `json:"user_code"`
	VerifyURL         string `json:"verify_url"`
	VerifyURLComplete string `json:"verify_url_complete"`
	ExpiresIn         int    `json:"expires_in"`
}

// Google Auth DTOs

type GoogleAuthStatusResponse struct {
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	Connected  bool   `json:"connected"`
	AuthMethod string `json:"auth_method,omitempty"`
	Email      string `json:"email,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

type StartGoogleAuthResponse struct {
	Status    string `json:"status"`
	VerifyURL string `json:"verify_url"`
}

type GoogleModelsRequest struct {
	AccessToken string `json:"access_token"`
	ProjectID   string `json:"project_id"`
}

type GoogleModelsResponse struct {
	Provider string             `json:"provider"`
	Source   string             `json:"source"`
	Models   []GoogleModelItem  `json:"models"`
}

type GoogleModelItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	IsExhausted bool   `json:"is_exhausted,omitempty"`
}
