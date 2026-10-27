package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// WarpService provides business logic for Cloudflare WARP integration.
type WarpService struct{ SettingService }

func (s *WarpService) GetWarpData() (string, error) { return s.GetWarp() }
func (s *WarpService) DelWarpData() error           { return s.SetWarp("") }

// Provider errors must never be treated as new credentials or a saved license.
// Bound both the request lifetime and response size; do not expose response bodies
// in errors because successful provider responses contain account credentials.
func warpRequest(method, endpoint, token string, payload any) (map[string]any, []byte, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
	}
	req, err := http.NewRequest(method, "https://api.cloudflareclient.com/v0a2158/reg"+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CF-Client-Version", "a-7.21-0721")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("WARP request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("WARP provider returned HTTP %d", resp.StatusCode)
	}
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > limit {
		return nil, nil, fmt.Errorf("WARP response is too large")
	}
	var data map[string]any
	if err = json.Unmarshal(raw, &data); err != nil || len(data) == 0 {
		return nil, nil, fmt.Errorf("WARP provider returned an invalid response")
	}
	if success, ok := data["success"].(bool); ok && !success {
		return nil, nil, fmt.Errorf("WARP provider rejected the request")
	}
	if errors, ok := data["errors"].([]any); ok && len(errors) > 0 {
		return nil, nil, fmt.Errorf("WARP provider rejected the request")
	}
	return data, raw, nil
}

func (s *WarpService) warpCredentials() (map[string]string, error) {
	raw, err := s.GetWarp()
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err = json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	if data["device_id"] == "" || data["access_token"] == "" {
		return nil, fmt.Errorf("WARP account credentials are missing")
	}
	return data, nil
}

func validateWarpConfig(data map[string]any) error {
	config, ok := data["config"].(map[string]any)
	if !ok {
		return fmt.Errorf("WARP response is missing its configuration")
	}
	peers, ok := config["peers"].([]any)
	if !ok || len(peers) == 0 {
		return fmt.Errorf("WARP response is missing its peers")
	}
	return nil
}

func (s *WarpService) GetWarpConfig() (string, error) {
	account, err := s.warpCredentials()
	if err != nil {
		return "", err
	}
	data, raw, err := warpRequest(http.MethodGet, "/"+url.PathEscape(account["device_id"]), account["access_token"], nil)
	if err != nil {
		return "", err
	}
	if err = validateWarpConfig(data); err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *WarpService) RegWarp(secretKey, publicKey string) (string, error) {
	hostName, _ := os.Hostname()
	payload := map[string]string{"key": publicKey, "tos": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), "type": "PC", "model": "vpn-ui", "name": hostName}
	data, raw, err := warpRequest(http.MethodPost, "", "", payload)
	if err != nil {
		return "", err
	}
	deviceID, _ := data["id"].(string)
	token, _ := data["token"].(string)
	account, _ := data["account"].(map[string]any)
	license, ok := account["license"].(string)
	if deviceID == "" || token == "" || !ok {
		return "", fmt.Errorf("WARP registration response is missing account credentials")
	}
	if err = validateWarpConfig(data); err != nil {
		return "", err
	}
	warpData, err := json.MarshalIndent(map[string]string{"access_token": token, "device_id": deviceID, "license_key": license, "private_key": secretKey}, "", "  ")
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(map[string]json.RawMessage{"data": warpData, "config": raw})
	if err != nil {
		return "", err
	}
	if err = s.SetWarp(string(warpData)); err != nil {
		return "", err
	}
	return string(result), nil
}

func (s *WarpService) SetWarpLicense(license string) (string, error) {
	account, err := s.warpCredentials()
	if err != nil {
		return "", err
	}
	data, _, err := warpRequest(http.MethodPut, "/"+url.PathEscape(account["device_id"])+"/account", account["access_token"], map[string]string{"license": license})
	if err != nil {
		return "", err
	}
	_, hasLicense := data["license"].(string)
	if !hasLicense && data["success"] != true {
		return "", fmt.Errorf("WARP license response is incomplete")
	}
	account["license_key"] = license
	raw, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return "", err
	}
	if err = s.SetWarp(string(raw)); err != nil {
		return "", err
	}
	return string(raw), nil
}
