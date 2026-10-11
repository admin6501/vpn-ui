// Package nodewire defines the versioned, private panel-to-node protocol.
package nodewire

import "encoding/json"
import "github.com/mhsanaei/3x-ui/v2/database/model"

const Version = 1
const LeaseSeconds = 90

type Enrollment struct {
	PairToken    string `json:"pairToken"`
	AgentToken   string `json:"agentToken"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
}
type EnrollmentReply struct {
	NodeID int `json:"nodeId"`
}
type Account struct {
	Account             model.Account `json:"account"`
	ProtocolCredentials string        `json:"protocolCredentials,omitempty"`
	ReportedUsage       int64         `json:"reportedUsage"` // Raw cumulative usage acknowledged by the panel.
	Remaining           int64         `json:"remaining"`     // -1 is unlimited; zero means exhausted.
	Allowed             bool          `json:"allowed"`
}
type ConfigCommand struct {
	Reset   bool   `json:"reset"`
	Core    string `json:"core"`
	Inbound int    `json:"inbound"`
	File    string `json:"file"`
	Text    string `json:"text"`
	Inspect bool   `json:"inspect"`
}
type CoreAction struct {
	Core   string `json:"core"`
	Action string `json:"action"`
}
type Snapshot struct {
	CoreActionGeneration int64                  `json:"coreActionGeneration"`
	CoreActionCommand    *CoreAction            `json:"coreActionCommand,omitempty"`
	ControlGeneration    int64                  `json:"controlGeneration"`
	ConfigGeneration     int64                  `json:"configGeneration"`
	ConfigCommand        *ConfigCommand         `json:"configCommand,omitempty"`
	Version              int                    `json:"version"`
	NodeID               int                    `json:"nodeId"`
	Enable               bool                   `json:"enable"`
	LeaseSeconds         int                    `json:"leaseSeconds"`
	Inbounds             []model.Inbound        `json:"inbounds"`
	Accounts             []Account              `json:"accounts"`
	Memberships          []model.AccountInbound `json:"memberships"`
	Template             string                 `json:"template"`
	CoreGeneration       int64                  `json:"coreGeneration"`
	RequestedCores       []string               `json:"requestedCores"`
	Files                map[string][]byte      `json:"files"`
}
type Counter struct {
	Kind      string `json:"kind"` // account (bill), membership (display), inbound (display)
	AccountID int    `json:"accountId"`
	InboundID int    `json:"inboundId"`
	Up        int64  `json:"up"`
	Down      int64  `json:"down"`
}
type Online struct {
	AccountID int `json:"accountId"`
	InboundID int `json:"inboundId"`
}
type Report struct {
	Version    int             `json:"version"`
	Sequence   int64           `json:"sequence"`
	Counters   []Counter       `json:"counters"`
	Online     []Online        `json:"online"`
	CoreReport json.RawMessage `json:"coreReport"`
	LastError  string          `json:"lastError"`
}
