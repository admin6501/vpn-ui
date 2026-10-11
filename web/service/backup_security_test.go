package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityBackupCopiesKeepPrivateDataAndExecutableModes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode, want os.FileMode
	}{
		{"vpn-ui.db", 0644, 0600}, {"vpn-ui.db-wal", 0644, 0600}, {"vpn-ui.db-shm", 0644, 0600}, {"vpn-ui-amd64", 0755, 0755},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src, dst := filepath.Join(dir, tc.name), filepath.Join(dir, "copy")
			if err := os.WriteFile(src, []byte("test-data"), tc.mode); err != nil {
				t.Fatal(err)
			}
			// Explicit permissions avoid dependence on the host umask.
			if err := os.Chmod(src, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, []byte("old-data"), 0666); err != nil {
				t.Fatal(err)
			}
			if err := CopyFile(src, dst); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(dst)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != tc.want {
				t.Fatalf("copied permissions: %o; want %o", info.Mode().Perm(), tc.want)
			}
			data, err := os.ReadFile(dst)
			if err != nil || string(data) != "test-data" {
				t.Fatal("copy contents changed")
			}
		})
	}
}
