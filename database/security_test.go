package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityDatabaseAndSidecarsArePrivate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "private", "vpn-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(dbPath + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s permissions: %o", suffix, info.Mode().Perm())
		}
		if err := os.Chmod(dbPath+suffix, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(dbPath + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("legacy %s permissions not repaired: %o", suffix, info.Mode().Perm())
		}
	}
}
