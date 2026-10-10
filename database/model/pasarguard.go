package model

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/compat/pasarguard"
	"time"
)

// PasarGuardSubscription binds the original identity to the current account.
// Original links continue to resolve without enumerating their issue timestamps.
type PasarGuardSubscription struct {
	SourceUserID  int        `gorm:"primaryKey" json:"-"`
	Username      string     `gorm:"uniqueIndex" json:"-"`
	AccountID     int        `gorm:"index" json:"-"`
	SubIDAtImport string     `json:"-"`
	CreatedAt     time.Time  `json:"-"`
	RevokedAt     *time.Time `json:"-"`
}

type PasarGuardSecret struct {
	ID         int    `gorm:"primaryKey" json:"-"`
	SigningKey string `json:"-"`
}

type RepresentativeRole struct {
	ID                          int    `json:"id" gorm:"primaryKey"`
	Name                        string `json:"name" gorm:"uniqueIndex"`
	IsOwner                     bool   `json:"is_owner"`
	Permissions                 string `json:"permissions"`
	Limits                      string `json:"limits"`
	Features                    string `json:"features"`
	Access                      string `json:"access"`
	DisabledWhenLimited         bool   `json:"disabled_when_limited"`
	DisconnectUsersWhenLimited  bool   `json:"disconnect_users_when_limited"`
	DisconnectUsersWhenDisabled bool   `json:"disconnect_users_when_disabled"`
}

// Retain the complete original host, group and core settings for migration
// review and future adapters; subscription endpoints are also mapped natively.
type PasarGuardResource struct {
	Kind     string `gorm:"primaryKey" json:"kind"`
	SourceID int    `gorm:"primaryKey" json:"sourceId"`
	Data     string `json:"data"`
}

func (r *RepresentativeRole) Policy() pasarguard.Policy {
	p := pasarguard.Policy{}
	if r == nil {
		return p
	}
	p.Owner = r.IsOwner
	p.DisabledWhenLimited = r.DisabledWhenLimited
	// Malformed permissions intentionally produce an empty policy.
	if json.Unmarshal([]byte(r.Permissions), &p.Permissions) != nil {
		p.Permissions = nil
	}
	return p
}
