package service

import (
	"testing"
	"time"
)

// The Clients table is paged by the SERVER, so its order is decided here and the
// browser only ticks the menu item. Each ordering is COMPLETE - "newest" already
// says which way it runs - so there is no direction to test, only five answers.

func names(rows []AccountRow) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].Email)
	}
	return out
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Four accounts made in a known order, one of them disabled and one of them online,
// so every ordering has something to separate.
func sortFixture() ([]AccountRow, map[int]int64, map[string]bool) {
	rows := []AccountRow{
		{Id: 1, Email: "alice", Enable: true},
		{Id: 2, Email: "bob", Enable: false},
		{Id: 3, Email: "carol", Enable: true},
		{Id: 4, Email: "dave", Enable: false},
	}
	createdAt := map[int]int64{1: 100, 2: 200, 3: 300, 4: 400}
	online := map[string]bool{"carol": true, "bob": true}
	return rows, createdAt, online
}

// An unknown or empty key must not error and must not leave the list in whatever
// order the database handed it over in. It falls back to newest, and says so,
// because the menu ticks its item from the echoed value.
func TestSortAccountRowsFallsBackToNewest(t *testing.T) {
	for _, key := range []string{"", "not-a-sort", "email", "quota"} {
		rows, createdAt, online := sortFixture()
		got := sortAccountRows(rows, createdAt, online, key)
		if got != AccountSortNewest {
			t.Errorf("key %q should normalise to newest, got %q", key, got)
		}
		if names(rows)[0] != "dave" {
			t.Errorf("key %q did not apply the newest ordering: %v", key, names(rows))
		}
	}
}

func TestSortAccountRowsByAge(t *testing.T) {
	rows, createdAt, online := sortFixture()
	sortAccountRows(rows, createdAt, online, AccountSortNewest)
	if got := names(rows); !equal(got, []string{"dave", "carol", "bob", "alice"}) {
		t.Errorf("newest first: %v", got)
	}

	rows, createdAt, online = sortFixture()
	sortAccountRows(rows, createdAt, online, AccountSortOldest)
	if got := names(rows); !equal(got, []string{"alice", "bob", "carol", "dave"}) {
		t.Errorf("oldest first: %v", got)
	}
}

// Accounts created in the same millisecond still come back in the order they were
// made, because the tie-break is the monotonic id and not the email. Without a total
// order the page slice is unstable and paging can show one account twice.
func TestSortAccountRowsByAgeTieBreaksOnId(t *testing.T) {
	rows := []AccountRow{
		{Id: 2, Email: "zoe"}, {Id: 1, Email: "adam"}, {Id: 3, Email: "mike"},
	}
	same := map[int]int64{1: 500, 2: 500, 3: 500}
	sortAccountRows(rows, same, nil, AccountSortNewest)
	if got := names(rows); !equal(got, []string{"mike", "zoe", "adam"}) {
		t.Errorf("newest with identical timestamps should fall back to id desc: %v", got)
	}

	rows = []AccountRow{{Id: 2, Email: "zoe"}, {Id: 1, Email: "adam"}, {Id: 3, Email: "mike"}}
	sortAccountRows(rows, same, nil, AccountSortOldest)
	if got := names(rows); !equal(got, []string{"adam", "zoe", "mike"}) {
		t.Errorf("oldest with identical timestamps should fall back to id asc: %v", got)
	}
}

// Connected right now first, then everyone else alphabetically. Deliberately not a
// health ordering: an account can be online and nearly out of data at the same time.
func TestSortAccountRowsByOnline(t *testing.T) {
	rows, createdAt, online := sortFixture()
	sortAccountRows(rows, createdAt, online, AccountSortOnline)
	if got := names(rows); !equal(got, []string{"bob", "carol", "alice", "dave"}) {
		t.Errorf("online first, then alphabetical: %v", got)
	}
}

// The two halves of one question. Disabled-first is the more useful of the pair,
// because the panel switches accounts off by itself when they expire or run out.
func TestSortAccountRowsByEnableState(t *testing.T) {
	rows, createdAt, online := sortFixture()
	sortAccountRows(rows, createdAt, online, AccountSortEnabled)
	if got := names(rows); !equal(got, []string{"alice", "carol", "bob", "dave"}) {
		t.Errorf("enabled first: %v", got)
	}

	rows, createdAt, online = sortFixture()
	sortAccountRows(rows, createdAt, online, AccountSortDisabled)
	if got := names(rows); !equal(got, []string{"bob", "dave", "alice", "carol"}) {
		t.Errorf("disabled first: %v", got)
	}
}

// Email comparison goes through accountKey, the case-insensitive trimmed form the
// rest of the accounts layer identifies an account by. Sorting on the raw string
// would put every capitalised address in a block of its own.
func TestSortAccountRowsTieBreakIsCaseInsensitive(t *testing.T) {
	rows := []AccountRow{
		{Id: 1, Email: "Bravo", Enable: true},
		{Id: 2, Email: "alpha", Enable: true},
		{Id: 3, Email: "Charlie", Enable: true},
	}
	sortAccountRows(rows, map[int]int64{}, nil, AccountSortEnabled)
	if got := names(rows); !equal(got, []string{"alpha", "Bravo", "Charlie"}) {
		t.Errorf("the tie-break should ignore case: %v", got)
	}
}

func TestAccountSortNameIsCaseInsensitiveAndStable(t *testing.T) {
	rows := []AccountRow{{Id: 3, Email: "zulu"}, {Id: 2, Email: "Alpha"}, {Id: 1, Email: "alpha"}}
	if got := sortAccountRows(rows, nil, nil, AccountSortName); got != AccountSortName {
		t.Fatal(got)
	}
	if rows[0].Id != 1 || rows[1].Id != 2 || rows[2].Id != 3 {
		t.Fatalf("unexpected name order: %+v", rows)
	}
}

func TestAccountSortTrafficAndDays(t *testing.T) {
	rows := []AccountRow{{Id: 1, Email: "light", TotalGB: 20, Up: 5, Down: 10}, {Id: 2, Email: "heavy", TotalGB: 1000, Up: 2, Down: 100}, {Id: 3, Email: "middle", TotalGB: 100, Up: 30, Down: 10}}
	if got := sortAccountRows(rows, nil, nil, AccountSortTraffic); got != AccountSortTraffic || !equal(names(rows), []string{"light", "middle", "heavy"}) {
		t.Fatalf("traffic: %v", names(rows))
	}
	now := time.Now().UnixMilli()
	rows = []AccountRow{{Id: 1, Email: "unlimited"}, {Id: 2, Email: "later", ExpiryTime: now + 86400000*5}, {Id: 3, Email: "expired", ExpiryTime: now - 86400000}, {Id: 4, Email: "delayed", ExpiryTime: -86400000 * 2}, {Id: 5, Email: "soon", ExpiryTime: now + 86400000}}
	if got := sortAccountRows(rows, nil, nil, AccountSortDays); got != AccountSortDays || !equal(names(rows), []string{"expired", "soon", "delayed", "later", "unlimited"}) {
		t.Fatalf("days: %v", names(rows))
	}
}

func TestAccountSortTrafficQuotaEdges(t *testing.T) {
	rows := []AccountRow{
		{Id: 1, Email: "unlimited", Up: 10000},
		{Id: 2, Email: "empty", TotalGB: 100},
		{Id: 3, Email: "over", TotalGB: 100, Up: 200},
		{Id: 4, Email: "full", TotalGB: 100, Down: 100},
		{Id: 5, Email: "large", TotalGB: 1<<63 - 1, Up: 1 << 62},
		{Id: 6, Email: "half", TotalGB: 100, Up: 50},
	}
	sortAccountRows(rows, nil, nil, AccountSortTraffic)
	if !equal(names(rows), []string{"full", "over", "large", "half", "empty", "unlimited"}) {
		t.Fatal(names(rows))
	}
}
