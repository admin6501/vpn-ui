package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestMtprotoMetricsKeepIdleSessionOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "telemt_user_connections_current{user=\"idle\"} 2\ntelemt_user_connections_current{user=\"closed\"} 0\ntelemt_user_octets_from_client{user=\"idle\"} 40\ntelemt_user_octets_to_client{user=\"idle\"} 50\n")
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	up, down, live := (&MtprotoService{}).scrapeMetricsState(port)
	if !live["idle"] || live["closed"] || up["idle"] != 40 || down["idle"] != 50 {
		t.Fatal(up, down, live)
	}
}
