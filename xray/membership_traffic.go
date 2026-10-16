package xray

import (
	"encoding/base64"
	"regexp"
	"strings"

	statsService "github.com/xtls/xray-core/app/stats/command"
)

var clientTrafficRegex = regexp.MustCompile(`^user>>>([^>]+)>>>traffic>>>(downlink|uplink)$`)
var membershipTrafficRegex = regexp.MustCompile(`^membership>>>([A-Za-z0-9_-]*\.[A-Za-z0-9_-]+)>>>traffic>>>(downlink|uplink)$`)

type trafficDirections struct{ up, down bool }

// Detailed counters are authoritative for their account AND direction, including
// zero values. Standard user counters measure the SAME bytes but reset at a
// different instant inside QueryStats. Adding them, or treating their difference
// as missing traffic, would duplicate bytes across collection ticks.
// Old cores have no detailed counters and retain the existing account fallback.
func parseTrafficStats(stats []*statsService.Stat) (map[string]*Traffic, []*ClientTraffic) {
	tags := make(map[string]*Traffic)
	accounts := make(map[string]*ClientTraffic)
	members := make(map[string]*ClientTraffic)
	covered := make(map[string]trafficDirections)
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		if match := trafficStatRegex.FindStringSubmatch(stat.Name); len(match) == 4 {
			processTraffic(match, stat.Value, tags)
		} else if match := membershipTrafficRegex.FindStringSubmatch(stat.Name); len(match) == 3 && stat.Value >= 0 {
			tag64, email64, _ := strings.Cut(match[1], ".")
			tag, err := base64.RawURLEncoding.DecodeString(tag64)
			if err != nil {
				continue
			}
			email, err := base64.RawURLEncoding.DecodeString(email64)
			if err != nil || len(email) == 0 {
				continue
			}
			record := members[match[1]]
			if record == nil {
				record = &ClientTraffic{Email: string(email), InboundTag: string(tag), CoreCounted: true, SourceTagged: true}
				members[match[1]] = record
			}
			directions := covered[record.Email]
			if match[2] == "uplink" {
				record.Up = stat.Value
				directions.up = true
			} else {
				record.Down = stat.Value
				directions.down = true
			}
			covered[record.Email] = directions
		} else if match := clientTrafficRegex.FindStringSubmatch(stat.Name); len(match) == 3 {
			processClientTraffic(match, stat.Value, accounts)
		}
	}
	records := mapToSlice(members)
	for email, record := range accounts {
		directions := covered[email]
		if directions.up && directions.down {
			continue
		}
		if directions.up {
			record.Up = 0
		}
		if directions.down {
			record.Down = 0
		}
		records = append(records, record)
	}
	return tags, records
}
