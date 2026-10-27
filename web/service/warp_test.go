package service

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
)

type warpTestTransport func(*http.Request) (*http.Response, error)

func (f warpTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWarpProviderFailuresPreserveExistingAccount(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "warp.db")); err != nil {
		t.Fatal(err)
	}
	s := &WarpService{}
	const original = `{"device_id":"existing-device","access_token":"existing-token","private_key":"existing-key","license_key":"existing-license"}`
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate-limited", 429, `{"errors":[{"message":"rate limited"}]}`},
		{"missing-fields", 200, `{}`},
		{"rejected-without-details", 200, `{"success":false,"errors":[]}`},
		{"malformed-response", 200, `not json`},
	} {
		for _, method := range []string{"register", "get", "license"} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				if err := s.SetWarp(original); err != nil {
					t.Fatal(err)
				}
				http.DefaultTransport = warpTestTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host != "api.cloudflareclient.com" {
						t.Fatalf("unexpected provider host %s", r.URL.Host)
					}
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
				})
				defer func() {
					if p := recover(); p != nil {
						t.Errorf("provider failure caused panic: %v", p)
					}
				}()
				var result string
				var err error
				switch method {
				case "register":
					result, err = s.RegWarp("new-private", "new-public")
				case "get":
					result, err = s.GetWarpConfig()
				case "license":
					result, err = s.SetWarpLicense("new-license")
				}
				if err == nil || result != "" {
					t.Errorf("provider failure must return an error and no data, got result %q, error %v", result, err)
				}
				saved, err := s.GetWarp()
				if err != nil || saved != original {
					t.Errorf("existing account changed on failure: %v", err)
				}
			})
		}
	}
}

func TestWarpSuccessfulResponsesAndJSONEscaping(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "warp.db")); err != nil {
		t.Fatal(err)
	}
	s := &WarpService{}
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	const config = `{"peers":[{"public_key":"peer-key","endpoint":{"host":"example.test:2408"}}],"client_id":"AAAA","interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700::2"}}}`
	const secret = "private\"key\\"
	const license = "license\"quoted\\"
	http.DefaultTransport = warpTestTransport(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 20*time.Second {
			t.Error("provider request must have a bounded deadline")
		}
		body := ""
		switch r.Method {
		case http.MethodPost:
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["key"] != "public\"key" {
				t.Error("registration key was not safely JSON encoded")
			}
			body = `{"id":"device","token":"token","account":{"license":"initial"},"config":` + config + `}`
		case http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Error("missing account authorization")
			}
			body = `{"config":` + config + `}`
		case http.MethodPut:
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["license"] != license {
				t.Error("license was not safely JSON encoded")
			}
			encoded, _ := json.Marshal(map[string]string{"license": license})
			body = string(encoded)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	registered, err := s.RegWarp(secret, "public\"key")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data   map[string]string `json:"data"`
		Config json.RawMessage   `json:"config"`
	}
	if err = json.Unmarshal([]byte(registered), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data["private_key"] != secret || len(result.Config) == 0 {
		t.Fatal("registration lost credentials or configuration")
	}
	if _, err = s.GetWarpConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetWarpLicense(license); err != nil {
		t.Fatal(err)
	}
	saved, err := s.warpCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if saved["private_key"] != secret || saved["license_key"] != license {
		t.Fatal("license update changed credentials")
	}
}

func TestWarpRejectsOversizedResponse(t *testing.T) {
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = warpTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", (1<<20)+1)))}, nil
	})
	if _, _, err := warpRequest(http.MethodGet, "", "", nil); err == nil {
		t.Fatal("oversized provider response accepted")
	}
}
