package model

type PanelLoginEvent struct {
	Id       int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID   int    `json:"-" gorm:"not null;index"`
	Username string `json:"username" gorm:"not null"`
	IP       string `json:"ip" gorm:"not null"`
	OS       string `json:"os" gorm:"not null"`
	Kind     string `json:"kind" gorm:"not null"`
	At       int64  `json:"at" gorm:"not null;index"`
}

type PanelClientEvent struct {
	Id             int64                `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID      int                  `json:"-" gorm:"not null;index"`
	OwnerID        int                  `json:"-" gorm:"not null;index"`
	Owner          string               `json:"owner" gorm:"not null"`
	ActorID        int                  `json:"-" gorm:"not null;default:0;index"`
	Actor          string               `json:"actor,omitempty" gorm:"not null;default:''"`
	Email          string               `json:"email" gorm:"not null"`
	Kind           string               `json:"kind" gorm:"not null;index"`
	Changes        string               `json:"changes" gorm:"not null;default:''"`
	AuditOperation string               `json:"-" gorm:"not null;default:''"`
	Details        *ClientChangeDetails `json:"details,omitempty" gorm:"serializer:json;type:text"`
	At             int64                `json:"at" gorm:"not null;index"`
}

// ClientChangeDetails records quota, validity, usage and status for auditing.
type ClientChangeDetails struct {
	Created          *ClientCreation       `json:"created,omitempty"`
	Deleted          *ClientDeletion       `json:"deleted,omitempty"`
	Quota            *ClientValueChange    `json:"quota,omitempty"`
	Expiry           *ClientValueChange    `json:"expiry,omitempty"`
	RenewalPeriod    *ClientValueChange    `json:"renewalPeriod,omitempty"`
	Inbounds         []ClientInboundChange `json:"inbounds,omitempty"`
	AutoRenewEnabled *bool                 `json:"autoRenewEnabled,omitempty"`
	UsageReset       bool                  `json:"usageReset,omitempty"`
}

// A nil state means that the inbound was not assigned.
type ClientInboundChange struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Before   *bool  `json:"before"`
	After    *bool  `json:"after"`
}

type ClientDeletion struct {
	ClientCreation
	InitialQuota *int64 `json:"initialQuota,omitempty"`
	Upload       *int64 `json:"upload,omitempty"`
	Download     *int64 `json:"download,omitempty"`
	Lifetime     *int64 `json:"lifetime,omitempty"`
	Enabled      bool   `json:"enabled"`
}

type ClientCreation struct {
	Quota  int64 `json:"quota"`
	Expiry int64 `json:"expiry"`
	Period int64 `json:"period"`
}

type PanelClientHidden struct {
	UserID  int   `gorm:"primaryKey;autoIncrement:false"`
	EventID int64 `gorm:"primaryKey;autoIncrement:false;index"`
}

type ClientValueChange struct {
	Before int64 `json:"before"`
	After  int64 `json:"after"`
}

type PanelExpiry struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID int    `json:"-" gorm:"not null;uniqueIndex"`
	OwnerID   int    `json:"-" gorm:"not null;index"`
	Email     string `json:"email" gorm:"not null"`
}

type PanelActivityRead struct {
	UserID  int    `gorm:"primaryKey;autoIncrement:false"`
	Kind    string `gorm:"primaryKey"`
	Through int64  `gorm:"not null"`
}

// PanelPaymentReminder keeps the latest billable change independently of audit retention.
type PanelPaymentReminder struct {
	AccountID   int   `gorm:"primaryKey;autoIncrement:false"`
	Revision    int64 `gorm:"not null"`
	Kind        string
	At          int64
	ConfirmedAt int64 `gorm:"not null;default:0"`
	ConfirmedBy int   `gorm:"not null;default:0"`
}

// PanelTrash stores private snapshots. The Activity API exposes only metadata.
type PanelTrash struct {
	Id          int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	OwnerID     int    `json:"-" gorm:"not null;index:idx_panel_trash_scope,priority:1"`
	Kind        string `json:"kind" gorm:"not null;index:idx_panel_trash_scope,priority:2"`
	Name        string `json:"name" gorm:"not null"`
	ClientCount int    `json:"clientCount"`
	DeletedAt   int64  `json:"deletedAt" gorm:"not null"`
	Payload     string `json:"-" gorm:"not null"`
}
