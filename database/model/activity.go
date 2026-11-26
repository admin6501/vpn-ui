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
	Email          string               `json:"email" gorm:"not null"`
	Kind           string               `json:"kind" gorm:"not null;index"`
	Changes        string               `json:"changes" gorm:"not null;default:''"`
	AuditOperation string               `json:"-" gorm:"not null;default:''"`
	Details        *ClientChangeDetails `json:"details,omitempty" gorm:"serializer:json;type:text"`
	At             int64                `json:"at" gorm:"not null;index"`
}

// ClientChangeDetails holds only the quota and period information safe to
// retain in an audit log. Credentials and free-form account notes stay out.
type ClientChangeDetails struct {
	Created       *ClientCreation    `json:"created,omitempty"`
	Quota         *ClientValueChange `json:"quota,omitempty"`
	Expiry        *ClientValueChange `json:"expiry,omitempty"`
	RenewalPeriod *ClientValueChange `json:"renewalPeriod,omitempty"`
	UsageReset    bool               `json:"usageReset,omitempty"`
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
