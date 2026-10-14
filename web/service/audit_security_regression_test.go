package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityAuditOpenVPNRejectsDirectiveInjection(t *testing.T) {
	on, off := true, false
	base := openvpnSettings{UdpEnable: &on, TcpEnable: &off, TlsUseFile: &on, TlsCryptEnable: &off, CaCertFile: "/etc/audit/ca.crt", ServerCertFile: "/etc/audit/server.crt", ServerKeyFile: "/etc/audit/key.pem"}
	for _, bad := range []string{"8.8.8.8\"\nsetenv AUDIT injected\n#", "8.8.8.8\rsetenv AUDIT x", "8.8.8.8\x00", "not-an-ip"} {
		st := base
		st.Dns1 = bad
		raw, _ := json.Marshal(st)
		row := &model.Inbound{Id: 1, Port: 1194, Protocol: model.OPENVPN, Settings: string(raw)}
		if (&InboundService{}).validateInboundConfig(row) == nil {
			t.Fatalf("write validator accepted %q", bad)
		}
		cfg, err := (&OpenVpnService{}).buildServerConfig(row, &st, "udp", 1194, "/opt/vpn-ui/vpn-ui")
		if err == nil || cfg != "" {
			t.Fatal("restored database bypassed renderer validation")
		}
	}
	for _, bad := range []string{"/tmp/cert\nsetenv AUDIT injected", "/tmp/key\rpath", "/tmp/key\x00suffix"} {
		st := base
		st.ServerKeyFile = bad
		if validateOpenVPNScalars(&st) == nil {
			t.Fatal("control characters accepted in a path")
		}
	}
	base.Dns1, base.Dns2 = "1.1.1.1", "2001:4860:4860::8888"
	base.CaCertFile = "/tmp/cert directory/ca.crt"
	base.ServerKeyFile = "/tmp/key\"with\\quote.pem"
	for _, proto := range []string{"tcp", "udp"} {
		cfg, err := (&OpenVpnService{}).buildServerConfig(&model.Inbound{Id: 1}, &base, proto, 1194, "/opt/vpn-ui/vpn-ui")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{`push "dhcp-option DNS 1.1.1.1"`, `push "dhcp-option DNS 2001:4860:4860::8888"`, `ca "/tmp/cert directory/ca.crt"`, `key "/tmp/key\"with\\quote.pem"`} {
			if !strings.Contains(cfg, line) {
				t.Fatalf("valid config missing %q", line)
			}
		}
	}
}

func TestSecurityAuditNodeOnlyTransfersCertificateFields(t *testing.T) {
	directory := t.TempDir()
	secret := filepath.Join(directory, "ordinary-private-file")
	if err := os.WriteFile(secret, []byte("AUDIT-NON-CERTIFICATE-SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, row := range []*model.Inbound{
		{Protocol: model.VLESS, Settings: `{"unrelated":{"keyFile":"` + secret + `"}}`},
		{Protocol: model.OPENVPN, Settings: `{"tlsUseFile":false,"serverKeyFile":"` + secret + `"}`},
		{Protocol: model.VLESS, StreamSettings: `{"security":"reality","tlsSettings":{"certificates":[{"keyFile":"` + secret + `"}]}}`},
	} {
		files := map[string][]byte{}
		if err := snapshotCertificateFiles(row, files); err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Fatal("inactive or unrelated field copied a host file")
		}
	}
	row := &model.Inbound{Id: 1, Protocol: model.VLESS, StreamSettings: `{"security":"tls","tlsSettings":{"certificates":[{"keyFile":"` + secret + `"}]}}`}
	original := row.StreamSettings
	files := map[string][]byte{}
	if snapshotCertificateFiles(row, files) == nil || len(files) != 0 || row.StreamSettings != original {
		t.Fatal("non-PEM file or partial bundle accepted")
	}
	material := makeTestCert(t, testCertOpts{sans: []string{"node.example.test"}, chain: true})
	cert, key := filepath.Join(directory, "fullchain.pem"), filepath.Join(directory, "key.pem")
	os.WriteFile(cert, material.certPEM, 0600)
	os.WriteFile(key, material.keyPEM, 0600)
	link := filepath.Join(directory, "active-key.pem")
	os.Symlink(key, link)
	raw, _ := json.Marshal(map[string]any{"security": "tls", "tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": link}}}})
	row.StreamSettings = string(raw)
	if err := snapshotCertificateFiles(row, files); err != nil {
		t.Fatal("valid renewable certificate rejected:", err)
	}
	if len(files) != 2 || !strings.Contains(row.StreamSettings, "nodefile:") {
		t.Fatal("valid certificate pair not transferred")
	}
}

func TestSecurityAuditNodeFileReadIsBoundedAndRegular(t *testing.T) {
	directory := t.TempDir()
	huge := filepath.Join(directory, "huge.pem")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(maxNodeCertificateBytes + 1)
	f.Close()
	fifo := filepath.Join(directory, "pipe.pem")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{huge, fifo, directory} {
		if _, err := readNodeCertificate(path, "key"); err == nil {
			t.Fatalf("nonregular or oversized file accepted: %s", path)
		}
	}
	cert := makeTestCert(t, testCertOpts{sans: []string{"audit.test"}})
	for _, data := range [][]byte{append([]byte("secret-prefix\n"), cert.certPEM...), append(cert.certPEM, []byte("secret-trailer")...), append([]byte("-----BEGIN CERTIFICATE-----\nsecret-prefix\n-----END CERTIFICATE-----\n"), cert.certPEM...)} {
		if validateNodeCertificateMaterial(data, "cert") == nil {
			t.Fatal("extra non-PEM material accepted")
		}
	}
}

func TestSecurityAuditCertificatePathAuthorization(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	user := &model.User{Enable: true, IsSuperAdmin: true}
	row := &model.Inbound{Protocol: model.VLESS, StreamSettings: `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/external/cert.pem","keyFile":"/external/key.pem"}]}}`}
	if ValidateInboundCertificatePaths(user, nil, row) == nil {
		t.Fatal("non-owner super admin introduced a host file path")
	}
	if err := ValidateInboundCertificatePaths(user, row, row); err != nil {
		t.Fatal("unchanged existing binding rejected:", err)
	}
	owner := *user
	owner.IsPanelOwner = true
	if err := ValidateInboundCertificatePaths(&owner, nil, row); err != nil {
		t.Fatal("owner's external certificate rejected:", err)
	}
	inline := &model.Inbound{Protocol: model.VLESS, StreamSettings: `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["inline-cert"],"key":["inline-key"]}]}}`}
	if err := ValidateInboundCertificatePaths(user, nil, inline); err != nil {
		t.Fatal("inline certificates incorrectly blocked:", err)
	}
}

func TestSecurityAuditManagedCertificateRemainsAssignable(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	store, err := OpenSSLStore(DefaultSSLStoreRoot())
	if err != nil {
		t.Fatal(err)
	}
	material := makeTestCert(t, testCertOpts{sans: []string{"node.example.test"}})
	version, err := store.Stage("audit", material.certPEM, material.keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(version); err != nil {
		t.Fatal(err)
	}
	stream, _ := json.Marshal(map[string]any{"security": "tls", "tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": store.ActiveCertPath(), "keyFile": store.ActiveKeyPath()}}}})
	row := &model.Inbound{Protocol: model.VLESS, StreamSettings: string(stream)}
	if err := ValidateInboundCertificatePaths(&model.User{Enable: true}, nil, row); err != nil {
		t.Fatal("managed picker certificate blocked:", err)
	}
	if err := snapshotCertificateFiles(row, map[string][]byte{}); err != nil {
		t.Fatal("active managed symlinks not transferred:", err)
	}
}
