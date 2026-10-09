package pasarguard

import (
	"encoding/json"
	"testing"
)

func TestScopedPolicy(t *testing.T) {
	var permissions map[string]map[string]json.RawMessage
	json.Unmarshal([]byte(`{"users":{"read":{"scope":1},"update":{"scope":1},"delete":{"scope":0},"create":true}}`), &permissions)
	p := Policy{Permissions: permissions}
	if !p.Allows("users", "update", false, 1, 1) || p.Allows("users", "update", false, 1, 2) {
		t.Fatal("ownership boundary failed")
	}
	if p.Allows("users", "delete", false, 1, 1) || p.Allows("settings", "update", false, 1, 1) {
		t.Fatal("missing/zero permission allowed")
	}
	if !p.Allows("users", "read", true, 1, 1) || p.Allows("users", "update", true, 1, 1) {
		t.Fatal("limited admin policy failed")
	}
	p.DisabledWhenLimited = true
	if p.Allows("users", "read", true, 1, 1) {
		t.Fatal("blocked limited admin allowed")
	}
	p.Owner = true
	if !p.Allows("settings", "update", true, 1, 2) {
		t.Fatal("owner bypass failed")
	}
}
