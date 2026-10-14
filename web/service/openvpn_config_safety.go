package service

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode"
)

// These values are single config tokens, never inline PEM blocks. Validate on
// both the write path and the renderer so imported/old databases cannot inject
// a new daemon directive.
func validateOpenVPNScalars(s *openvpnSettings) error {
	for _, dns := range []string{s.Dns1, s.Dns2} {
		if dns == "" {
			continue
		}
		if _, err := netip.ParseAddr(dns); err != nil {
			return fmt.Errorf("OpenVPN DNS must be an IPv4 or IPv6 address")
		}
	}
	for _, path := range []string{s.CaCertFile, s.ServerCertFile, s.ServerKeyFile, s.TlsCryptFile} {
		if strings.ContainsFunc(path, func(r rune) bool { return unicode.IsControl(r) }) {
			return fmt.Errorf("OpenVPN certificate paths cannot contain control characters")
		}
	}
	return nil
}

// OpenVPN's lexer accepts quoted tokens with escaped quotes and backslashes.
// Keep simple paths unchanged for compatibility with existing configurations.
func openVPNToken(value string) string {
	if !strings.ContainsAny(value, " \t\"'\\#;") {
		return value
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
