package model

// ClientGroup is the authoritative, dynamic collection of inbounds selected by
// customers. Membership rows deliberately carry no credentials or quota.
type ClientGroup struct {
	ID           int    `gorm:"primaryKey" json:"id"`
	Name         string `json:"name"`
	PasarGuardID *int   `gorm:"uniqueIndex" json:"pasarGuardId,omitempty"`
	Disabled     bool   `gorm:"not null;default:0" json:"disabled"`
}

type GroupInbound struct {
	GroupID   int `gorm:"primaryKey" json:"groupId"`
	InboundID int `gorm:"primaryKey;index" json:"inboundId"`
}

type AccountGroup struct {
	AccountID int `gorm:"primaryKey" json:"accountId"`
	GroupID   int `gorm:"primaryKey;index" json:"groupId"`
}

// RepresentativeGroupAccess is the representative's explicit grant. The inbound
// scope is derived from these groups on every read, including after group edits.
type RepresentativeGroupAccess struct {
	UserID  int `gorm:"primaryKey" json:"userId"`
	GroupID int `gorm:"primaryKey;index" json:"groupId"`
}
