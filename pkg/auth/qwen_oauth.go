package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	qwenOAuthBaseURL         = "https://chat.qwen.ai"
	qwenOAuthClientID        = "f0304373b74a44d2b584a3fb70ca9e56"
	qwenOAuthScope           = "openid profile email model.completion"
	qwenOAuthDeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
	qwenOAuthDeviceCodeURL   = qwenOAuthBaseURL + "/api/v1/oauth2/device/code"
	qwenOAuthTokenURL        = qwenOAuthBaseURL + "/api/v1/oauth2/token"
	qwenOAuthAuthTypeHeader  = "qwen-oauth"
)

type QwenDevicePollStatus string

const (
	QwenDevicePollSuccess  QwenDevicePollStatus = "success"
	QwenDevicePollPending  QwenDevicePollStatus = "pending"
	QwenDevicePollSlowDown QwenDevicePollStatus = "slow_down"
)

type QwenDeviceCodeInfo struct {
	DeviceCode        string `json:"device_code"`
	UserCode          string `json:"user_code"`
	VerifyURL         string `json:"verification_uri"`
	VerifyURLComplete string `json:"verification_uri_complete"`
	ExpiresIn         int    `json:"expires_in"`
}

type qwenOAuthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type qwenTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	ResourceURL  string `json:"resource_url"`
	IDToken      string `json:"id_token"`
}

func QwenOAuthConfig() OAuthProviderConfig {
	return OAuthProviderConfig{
		Issuer:   qwenOAuthBaseURL,
		ClientID: qwenOAuthClientID,
		Scopes:   qwenOAuthScope,
	}
}

func RequestQwenDeviceCode(pkce PKCECodes) (*QwenDeviceCodeInfo, error) {
	data := url.Values{
		"client_id":             {qwenOAuthClientID},
		"scope":                 {qwenOAuthScope},
		"code_challenge":        {pkce.CodeChallenge},
		"code_challenge_method": {"S256"},
	}

	req, err := http.NewRequest("POST", qwenOAuthDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating qwen device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting qwen device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading qwen device code response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen device code request failed: %s", string(body))
	}

	var info QwenDeviceCodeInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parsing qwen device code response: %w", err)
	}
	if info.DeviceCode == "" || info.VerifyURLComplete == "" {
		return nil, fmt.Errorf("invalid qwen device code response")
	}
	return &info, nil
}

func PollQwenDeviceCodeOnce(deviceCode, codeVerifier string) (*AuthCredential, QwenDevicePollStatus, error) {
	data := url.Values{
		"grant_type":    {qwenOAuthDeviceGrantType},
		"client_id":     {qwenOAuthClientID},
		"device_code":   {deviceCode},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequest("POST", qwenOAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("creating qwen device token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("polling qwen device token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading qwen device token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var oauthErr qwenOAuthError
		if err := json.Unmarshal(body, &oauthErr); err == nil {
			switch {
			case resp.StatusCode == http.StatusBadRequest && oauthErr.Error == "authorization_pending":
				return nil, QwenDevicePollPending, nil
			case resp.StatusCode == http.StatusTooManyRequests && oauthErr.Error == "slow_down":
				return nil, QwenDevicePollSlowDown, nil
			case oauthErr.Error != "":
				return nil, "", fmt.Errorf(
					"qwen device token poll failed: %s - %s",
					oauthErr.Error,
					oauthErr.ErrorDescription,
				)
			}
		}
		return nil, "", fmt.Errorf(
			"qwen device token poll failed: status=%d body=%s",
			resp.StatusCode,
			string(body),
		)
	}

	cred, err := parseQwenTokenResponse(body)
	if err != nil {
		return nil, "", err
	}
	return cred, QwenDevicePollSuccess, nil
}

func RefreshQwenAccessToken(cred *AuthCredential) (*AuthCredential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {cred.RefreshToken},
		"client_id":     {qwenOAuthClientID},
	}

	req, err := http.NewRequest("POST", qwenOAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating qwen refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing qwen token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading qwen refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen token refresh failed: %s", string(body))
	}

	refreshed, err := parseQwenTokenResponse(body)
	if err != nil {
		return nil, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = cred.RefreshToken
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = cred.AccountID
	}
	if refreshed.Email == "" {
		refreshed.Email = cred.Email
	}
	if refreshed.ResourceURL == "" {
		refreshed.ResourceURL = cred.ResourceURL
	}
	return refreshed, nil
}

func parseQwenTokenResponse(body []byte) (*AuthCredential, error) {
	var tokenResp qwenTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing qwen token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in qwen response")
	}

	var expiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	cred := &AuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Provider:     "qwen",
		AuthMethod:   "oauth",
		ResourceURL:  tokenResp.ResourceURL,
	}

	if accountID := extractAccountID(tokenResp.IDToken); accountID != "" {
		cred.AccountID = accountID
	}

	return cred, nil
}
