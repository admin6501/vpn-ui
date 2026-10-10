package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOfflinePackageInstallDoesNotRunCommands(t *testing.T) {
	t.Setenv("VPNUI_OFFLINE", "1")
	marker := filepath.Join(t.TempDir(), "unexpected-download")
	pm := &packageManager{name: "unknown", refresh: []string{"touch", marker}, install: []string{"touch", marker}}
	_, err := pm.installPackage("missing-package")
	if err == nil || !strings.Contains(err.Error(), "offline mode") {
		t.Fatalf("missing prerequisite was not reported: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("offline mode executed a package command")
	}
}

func TestOfflinePublicIPAvoidsDiscovery(t *testing.T) {
	t.Setenv("VPNUI_OFFLINE", "1")
	t.Setenv("VPNUI_PUBLIC_IP", "")
	if got := GetServerIPv4(); got != "127.0.0.1" {
		t.Fatalf("fallback = %q", got)
	}
	t.Setenv("VPNUI_PUBLIC_IP", "203.0.113.12")
	if got := GetServerIPv4(); got != "203.0.113.12" {
		t.Fatalf("explicit address = %q", got)
	}
	t.Setenv("VPNUI_PUBLIC_IP", "invalid-address")
	if got := GetServerIPv4(); got != "127.0.0.1" {
		t.Fatalf("invalid address = %q", got)
	}
}

func TestOfflineDashboardPublicIPDoesNotFetch(t *testing.T) {
	t.Setenv("VPNUI_OFFLINE", "1")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("203.0.113.9"))
	}))
	defer server.Close()
	if got := getPublicIP(server.URL); got != "N/A" {
		t.Fatalf("offline discovery = %q", got)
	}
	if calls.Load() != 0 {
		t.Fatal("dashboard queried an IP-discovery endpoint in offline mode")
	}
}
