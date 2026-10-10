//go:build linux

package sys

import (
	"strings"
	"testing"
)

func TestSocketCountsExcludeHeadersAndClosedTCP(t *testing.T) {
	text := `  sl local_address rem_address st tx_queue
 0: 0100007F:1234 00000000:0000 0A 00000000:00000000
 1: 0100007F:1234 0200007F:5678 01 00000000:00000000
 2: 0100007F:1234 0200007F:5678 06 00000000:00000000
 3: 0100007F:1234 0200007F:5678 08 00000000:00000000
 4: 0100007F:1234 0200007F:5678 05 00000000:00000000
 malformed row
`
	count, err := countSocketRows(strings.NewReader(text), true)
	if err != nil || count != 1 {
		t.Fatalf("TCP: count %d error %v", count, err)
	}
	count, err = countSocketRows(strings.NewReader("sl local_address rem_address st\n0: 00000000:1234 00000000:0000 07\n1: 00000000:5678 0200007F:1234 01\n"), false)
	if err != nil || count != 2 {
		t.Fatalf("UDP sockets: count %d error %v", count, err)
	}
	count, err = countSocketRows(strings.NewReader("sl local_address rem_address st\n"), false)
	if err != nil || count != 0 {
		t.Fatalf("empty table: %d %v", count, err)
	}
}
