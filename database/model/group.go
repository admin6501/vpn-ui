package model

// ClientGroup is the authoritative, dynamic collection of inbounds selected by
// customers. Membership rows deliberately carry no credentials or quota.
type ClientGroup struct {
	ID   int    `gorm:"primaryKey" json:"id"`
	Name string `json:"name"`
}

type GroupInbound struct {
	GroupID   int `gorm:"primaryKey" json:"groupId"`
	InboundID int `gorm:"primaryKey;index" json:"inboundId"`
}

type AccountGroup struct {
	AccountID int `gorm:"primaryKey" json:"accountId"`
	GroupID   int `gorm:"primaryKey;index" json:"groupId"`
}
