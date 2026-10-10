package pasarguard

import "encoding/json"

// Policy follows PasarGuard's deny-by-default resource/action permissions.
// Scope 1 is OWN, scope 2 is ALL. A boolean true permits all scopes.
type Policy struct {
	Owner               bool
	DisabledWhenLimited bool
	Permissions         map[string]map[string]json.RawMessage
}

func (p Policy) Scope(resource, action string) int {
	if p.Owner {
		return 2
	}
	raw := p.Permissions[resource][action]
	var allowed bool
	if json.Unmarshal(raw, &allowed) == nil {
		if allowed {
			return 2
		}
		return 0
	}
	var scoped struct {
		Scope int `json:"scope"`
	}
	if json.Unmarshal(raw, &scoped) != nil || (scoped.Scope != 1 && scoped.Scope != 2) {
		return 0
	}
	return scoped.Scope
}

func (p Policy) Allows(resource, action string, limited bool, actorID, ownerID int) bool {
	if p.Owner {
		return true
	}
	if limited {
		if p.DisabledWhenLimited {
			return false
		}
		switch action {
		case "read", "read_simple", "read_general", "logs", "stats":
		default:
			return false
		}
	}
	scope := p.Scope(resource, action)
	return scope == 2 || scope == 1 && actorID == ownerID
}

type Limits struct {
	MaxUsers     *int   `json:"max_users"`
	DataLimitMin *int64 `json:"data_limit_min"`
	DataLimitMax *int64 `json:"data_limit_max"`
	ExpireMin    *int64 `json:"expire_min"`
	ExpireMax    *int64 `json:"expire_max"`
}
