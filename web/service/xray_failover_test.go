package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestCompileFailoverBalancer(t *testing.T) {
	cfg := &xray.Config{
		RouterConfig: []byte(`{"rules":[{"balancerTag":"edge","network":"tcp"}],"balancers":[
			{"tag":"plain","selector":["backup"],"strategy":{"type":"leastPing"}},
			{"tag":"edge","selector":["backup"],"primaryTag":"primary","strategy":{"type":"failover"}}]}`),
		OutboundConfigs: []byte(`[{"tag":"primary"},{"tag":"backup"},{"tag":"vpnui-failover-block"}]`),
		Observatory:     []byte(`{"subjectSelector":["another"],"probeInterval":"7s"}`),
	}
	if err := compileFailoverBalancers(cfg); err != nil {
		t.Fatal(err)
	}
	var routing struct {
		Balancers []struct {
			Selector    []string
			FallbackTag string
		}
	}
	var observer struct {
		SubjectSelector []string
		ProbeInterval   string
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(cfg.Observatory, &observer); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(routing.Balancers[0].Selector) != "[backup]" ||
		fmt.Sprint(routing.Balancers[1].Selector) != "[primary backup]" ||
		routing.Balancers[1].FallbackTag != "vpnui-failover-block-" ||
		fmt.Sprint(observer.SubjectSelector) != "[another primary backup]" || observer.ProbeInterval != "7s" {
		t.Fatalf("unexpected runtime configuration: %s / %s", cfg.RouterConfig, cfg.Observatory)
	}
}

func TestFailoverRoutesWithPatchedXray(t *testing.T) {
	binary := os.Getenv("VPNUI_TEST_FAILOVER_XRAY")
	if binary == "" {
		t.Skip("VPNUI_TEST_FAILOVER_XRAY is not set")
	}
	type path struct {
		server *httptest.Server
		delay  atomic.Int64
		down   atomic.Bool
	}
	outbounds := []map[string]any{{"tag": "default", "protocol": "blackhole"}}
	paths := make(map[string]*path)
	for _, tag := range []string{"primary", "primary-extra", "backup-a", "backup-b"} {
		p := &path{}
		p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/probe" {
				if p.down.Load() {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = conn.Close()
					}
					return
				}
				time.Sleep(time.Duration(p.delay.Load()) * time.Millisecond)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_, _ = fmt.Fprint(w, tag)
		}))
		t.Cleanup(p.server.Close)
		paths[tag] = p
		outbounds = append(outbounds, map[string]any{
			"tag": tag, "protocol": "freedom",
			"settings": map[string]string{"redirect": p.server.Listener.Addr().String()},
		})
	}
	paths["primary"].delay.Store(90)
	paths["backup-a"].delay.Store(20)
	paths["backup-b"].delay.Store(60)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	rawOutbounds, err := json.Marshal(outbounds)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &xray.Config{
		InboundConfigs: []xray.InboundConfig{{
			Tag: "client", Listen: []byte(`"127.0.0.1"`), Port: address.Port, Protocol: "socks",
			Settings: []byte(`{"auth":"noauth"}`),
		}},
		OutboundConfigs: rawOutbounds,
		RouterConfig: []byte(`{
			"rules":[{"type":"field","inboundTag":["client"],"balancerTag":"edge"}],
			"balancers":[{"tag":"edge","primaryTag":"primary","selector":["backup-a","backup-b"],"strategy":{"type":"failover"}}]}`),
		Observatory: []byte(`{"probeURL":"http://probe.invalid/probe","probeInterval":"100ms","enableConcurrency":true}`),
	}
	if err := compileFailoverBalancers(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "xray.json")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	process := exec.CommandContext(ctx, binary, "-c", filename)
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = process.Wait()
	}()
	transport := &http.Transport{
		DisableKeepAlives: true,
		Proxy:             http.ProxyURL(&url.URL{Scheme: "socks5", Host: address.String()}),
	}
	client := &http.Client{Transport: transport, Timeout: 350 * time.Millisecond}
	expect := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		var got string
		for time.Now().Before(deadline) {
			response, err := client.Get("http://target.invalid/traffic")
			got = ""
			if err == nil {
				body, _ := io.ReadAll(response.Body)
				_ = response.Body.Close()
				got = string(body)
			}
			if got == want {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("expected route %q, got %q", want, got)
	}
	expect("primary")
	paths["primary"].down.Store(true)
	expect("backup-a")
	paths["backup-a"].delay.Store(120)
	expect("backup-b")
	paths["primary"].down.Store(false)
	expect("primary")
	paths["primary"].down.Store(true)
	paths["backup-a"].down.Store(true)
	paths["backup-b"].down.Store(true)
	expect("")
}
