package xray

// ClientTraffic represents traffic statistics and limits for a specific client.
// It tracks upload/download usage, expiry times, and online status for inbound clients.
type ClientTraffic struct {
	// Collector-only liveness evidence; never charged or stored as quota.
	OnlineSnapshot bool   `json:"-" gorm:"-"`
	Online         bool   `json:"online,omitempty" gorm:"-"`
	InboundTag     string `json:"inboundTag,omitempty" gorm:"-"`
	// SourceTagged is a per-inbound byte counter from the patched core. An empty
	// or unrecognised tag stays unattributed, rather than using a guessed source.
	SourceTagged bool `json:"-" gorm:"-"`
	Id           int  `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	// InboundId is the account's HOME inbound: whichever one it was created on.
	//
	// It is not "the inbound this account is served on", and must not be used as
	// though it were. Email below is unique panel-wide, so there is exactly ONE
	// row per account no matter how many inbounds serve it, and this column can
	// only ever name one of them. Every enforcement path that scoped itself with
	// `WHERE inbound_id = ?` therefore acted on one arbitrary membership and left
	// the rest unenforced; they now resolve by email instead.
	//
	// Indexed because Inbound.ClientStats is a has-many on it and the preload runs
	// on every inbound list.
	InboundId int  `json:"inboundId" form:"inboundId" gorm:"index"`
	Enable    bool `json:"enable" form:"enable"`
	// Email is the global account identity, unique across ALL inbounds (see the email
	// helpers in web/service/inbound.go). This index is the last line of defense, not
	// a formality: ImportDB (web/service/server.go) swaps the SQLite file wholesale
	// and so bypasses every service-level check, leaving the constraint as the only
	// thing standing between a hand-edited backup and two clients sharing an identity.
	Email string `json:"email" form:"email" gorm:"unique"`
	UUID  string `json:"uuid" form:"uuid" gorm:"-"`
	SubId string `json:"subId" form:"subId" gorm:"-"`
	// Up/Down/AllTime are the ACCOUNT's usage, panel-wide, and mean exactly what they
	// always have. The quota is an account-level limit, so every "how full is this
	// client" question - the progress bars, the usage colours, the depletion sweep,
	// the remaining-bytes figure - has to compare against these and not against one
	// inbound's slice of them.
	Up      int64 `json:"up" form:"up"`
	Down    int64 `json:"down" form:"down"`
	AllTime int64 `json:"allTime" form:"allTime"`

	// InboundUp/InboundDown/InboundAllTime are the same account's usage THROUGH THE
	// ONE INBOUND this row was rendered under, read from its membership row
	// (model.AccountInbound). Additive: nothing that existed before this field reads
	// them, so no quota or enforcement behaviour depends on them.
	//
	// Display only, hence gorm:"-": the authoritative counter is still the single
	// client_traffics row above. Set by InboundService.attachClientStats.
	InboundUp      int64 `json:"inboundUp" gorm:"-"`
	InboundDown    int64 `json:"inboundDown" gorm:"-"`
	InboundAllTime int64 `json:"inboundAllTime" gorm:"-"`

	// Shared* is the pooled part INCLUDED in Inbound*, not additional usage.
	// Keeping it separate lets account views count that pool once and show each
	// membership's measured share as Inbound* minus Shared*. Display only.
	SharedUp      int64 `json:"sharedUp" gorm:"-"`
	SharedDown    int64 `json:"sharedDown" gorm:"-"`
	SharedAllTime int64 `json:"sharedAllTime" gorm:"-"`

	// Shared says Inbound* includes the pooled Shared* remainder alongside this
	// inbound's own measured share. The same pool can appear on several inbounds.
	//
	// Historical or legacy-core bytes without a source cannot be reconstructed
	// from account totals. Keep that pool separate from new measured shares.
	Shared bool `json:"shared" gorm:"-"`

	// CoreCounted marks a collected record counted by Xray. New cores include
	// SourceTagged/InboundTag; account-only records from old cores can still use
	// unambiguous membership attribution. Never persisted or used as quota itself.
	CoreCounted bool `json:"-" gorm:"-"`

	ExpiryTime int64 `json:"expiryTime" form:"expiryTime"`
	Total      int64 `json:"total" form:"total"`
	Reset      int   `json:"reset" form:"reset" gorm:"default:0"`
	LastOnline int64 `json:"lastOnline" form:"lastOnline" gorm:"default:0"`
}
