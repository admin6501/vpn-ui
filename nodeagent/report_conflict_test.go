package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeReportConflictDiagnosticsUseOnlyFixedCodes(t *testing.T) {
	for _, code := range []string{"counter_out_of_scope", "counter_regressed", "sequence_gap", "invalid_report", "private-key-fixture"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-VPN-UI-Node-Error", code)
			w.WriteHeader(409)
			w.Write([]byte("sensitive-body-fixture"))
		}))
		err := exchange(context.Background(), server.Client(), Config{PanelURL: server.URL + "/"}, "sync", nil, &struct{}{})
		server.Close()
		if err == nil {
			t.Fatal("conflict accepted")
		}
		if strings.Contains(err.Error(), "sensitive-body-fixture") || strings.Contains(err.Error(), "private-key-fixture") {
			t.Fatal("untrusted error body/header exposed")
		}
		if code != "private-key-fixture" && !strings.Contains(err.Error(), code) {
			t.Fatal("fixed conflict diagnostic lost", err)
		}
	}
}
