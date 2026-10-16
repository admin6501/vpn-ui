package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/gorm"
)

// Resolve the patched core's per-membership counters by account and exact tag.
// Legacy cores expose only account totals: keep the previous unambiguous-source
// fallback for those records. An unresolved exact tag is never replaced with a
// guess, and its bytes still reach the account total as unattributed usage.

// coreCandidate is one inbound an account's core-counted bytes could have come
// through.
type coreCandidate struct {
	Id       int
	Tag      string
	Protocol model.Protocol
	Email    string
}

// attributeCoreRecords maps exact traffic/liveness tags to current memberships.
// inboundTraffics provides the active-inbound fallback for legacy account stats.
// Lookup failures preserve collected bytes without assigning them to a guess.
func attributeCoreRecords(tx *gorm.DB, inboundTraffics []*xray.Traffic, records []*xray.ClientTraffic) {
	if tx == nil {
		return
	}

	// Only records that (a) came from the core, so their bytes really did enter
	// through an Xray-terminated inbound, and (b) name no inbound yet. A collector
	// that already knows its source is never second-guessed.
	pending := map[string][]*xray.ClientTraffic{}
	for _, record := range records {
		if record == nil || record.InboundId != 0 {
			continue
		}
		if record.SourceTagged || (record.Online && record.InboundTag != "") {
			pending[accountKey(record.Email)] = append(pending[accountKey(record.Email)], record)
			continue
		}
		if !record.CoreCounted {
			continue
		}
		if record.Up+record.Down <= 0 {
			// Nothing to attribute, and stamping it would light a membership's lamp
			// for an account that moved no bytes.
			continue
		}
		key := accountKey(record.Email)
		if key == "" {
			continue
		}
		pending[key] = append(pending[key], record)
	}
	if len(pending) == 0 {
		return
	}

	keys := make([]string, 0, len(pending))
	for key := range pending {
		keys = append(keys, key)
	}

	// The memberships, straight off account_inbounds rather than by scanning every
	// inbound's settings JSON: this runs on every traffic tick, and the join is two
	// indexed lookups where the scan is the whole table.
	var rows []coreCandidate
	err := tx.Table("account_inbounds").
		Select("inbounds.id AS id, inbounds.tag AS tag, inbounds.protocol AS protocol, "+
			"LOWER(TRIM(accounts.email)) AS email").
		Joins("JOIN accounts ON accounts.id = account_inbounds.account_id").
		Joins("JOIN inbounds ON inbounds.id = account_inbounds.inbound_id").
		Where("inbounds.node_id = ?", 0).
		Where("LOWER(TRIM(accounts.email)) IN (?)", keys).
		Scan(&rows).Error
	if err != nil {
		logger.Warning("per-inbound attribution: cannot resolve memberships, leaving this tick's core traffic unattributed: ", err)
		return
	}

	candidates := map[string][]coreCandidate{}
	for _, row := range rows {
		// Rule 1: a pool VPN inbound cannot have produced a user stat, so it is not a
		// candidate no matter what its own counters say.
		if isVpnProtocol(row.Protocol) {
			continue
		}
		candidates[row.Email] = append(candidates[row.Email], row)
	}

	// Rule 3's evidence: which inbounds moved anything at all this tick.
	active := map[string]bool{}
	for _, traffic := range inboundTraffics {
		if traffic != nil && traffic.IsInbound && traffic.Up+traffic.Down > 0 {
			active[traffic.Tag] = true
		}
	}

	for key, pendingRecords := range pending {
		inboundId := attributableInbound(candidates[key], active)
		for _, record := range pendingRecords {
			if record.SourceTagged || (record.Online && record.InboundTag != "") {
				// Validate both account membership and tag; never infer other memberships.
				for _, candidate := range candidates[key] {
					if record.InboundTag != "" && candidate.Tag == record.InboundTag {
						record.InboundId = candidate.Id
						break
					}
				}
				if record.InboundId == 0 {
					record.Online = false
				}
			} else if inboundId != 0 {
				record.InboundId = inboundId
			}
		}
	}
}

// ResolveCoreTrafficSources runs before merging relay fallbacks, so their exact
// source can be compared with the core's records without suppressing a different
// inbound of the same account. AddTraffic also resolves callers that collect
// traffic independently; records already carrying a source are left intact.
func (s *InboundService) ResolveCoreTrafficSources(inboundTraffics []*xray.Traffic, records []*xray.ClientTraffic) {
	attributeCoreRecords(database.GetDB(), inboundTraffics, records)
}

// attributableInbound applies rules 2 and 3 to one account's candidates, returning 0
// for "still ambiguous".
func attributableInbound(candidates []coreCandidate, active map[string]bool) int {
	switch len(candidates) {
	case 0:
		// No memberships to attribute with. Either the accounts layer has not been
		// migrated on this panel, or the client lives in an inbound's settings without
		// a membership row; both are the pre-existing unattributed case.
		return 0
	case 1:
		// Rule 2. Deliberately NOT also requiring the tag to be active: inbound-level
		// stats can be absent (a tick collected without them, an inbound whose totals
		// were reset in between), and that must not undo an answer that is already
		// forced by there being nothing else it could be.
		return candidates[0].Id
	}
	if len(active) == 0 {
		return 0
	}
	chosen := 0
	for _, candidate := range candidates {
		if !active[candidate.Tag] {
			continue
		}
		if chosen != 0 && chosen != candidate.Id {
			// Two of this account's inbounds were live this tick, so which of them
			// moved these bytes is genuinely unknown. Rule 3 stops here.
			return 0
		}
		chosen = candidate.Id
	}
	return chosen
}
