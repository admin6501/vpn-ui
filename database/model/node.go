package model

// RemoteNode has its own credential and an outbound-only agent. Secrets are
// hashes and are never serialized into an operator's ordinary node list.
type RemoteNode struct {
	CoreActionGeneration int64  `json:"coreActionGeneration" gorm:"not null;default:0"`
	CoreActionCommand    string `json:"-"`
	Paused               bool   `json:"paused" gorm:"not null;default:false"`
	ControlGeneration    int64  `json:"controlGeneration" gorm:"not null;default:0"`
	ConfigGeneration     int64  `json:"configGeneration" gorm:"not null;default:0"`
	ConfigCommand        string `json:"-"`
	ID                   int    `json:"id" gorm:"primaryKey"`
	Name                 string `json:"name"`
	Address              string `json:"address"`
	Enable               bool   `json:"enable"`
	PairHash             string `json:"-" gorm:"uniqueIndex"`
	PairExpiresAt        int64  `json:"-"`
	TokenHash            string `json:"-"`
	LastSeen             int64  `json:"lastSeen"`
	AgentVersion         string `json:"agentVersion"`
	Architecture         string `json:"architecture"`
	LastSequence         int64  `json:"-"`
	CoreGeneration       int64  `json:"coreGeneration"`
	RequestedCores       string `json:"-"`
	CoreReport           string `json:"-"`
	Online               string `json:"-"`
	LastError            string `json:"lastError"`
}

// NodeCounter stores the last accepted cumulative counter. Updating counters
// and the global account bill in one transaction makes retries idempotent.
type NodeCounter struct {
	NodeID    int    `gorm:"primaryKey;autoIncrement:false"`
	Kind      string `gorm:"primaryKey"`
	AccountID int    `gorm:"primaryKey;autoIncrement:false"`
	InboundID int    `gorm:"primaryKey;autoIncrement:false"`
	Up        int64
	Down      int64
}
