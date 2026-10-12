package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeProvisionPackageRefreshOnce(t *testing.T) {
	useTempOwnership(t)
	t.Setenv("VPNUI_OFFLINE", "0")
	dir := t.TempDir()
	script := filepath.Join(dir, "packages")
	log := filepath.Join(dir, "calls")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PACKAGE_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PACKAGE_TEST_LOG", log)
	pm := &packageManager{name: "test", refresh: []string{script, "refresh"}, install: []string{script, "install"}}
	for _, pkg := range []string{"dkms", "gcc", "make", "headers"} {
		if _, err := pm.installPackage(pkg); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	if strings.Count(calls, "refresh\n") != 1 || strings.Count(calls, "install ") != 4 {
		t.Fatalf("repeated refresh or skipped prerequisite: %s", calls)
	}
}

func TestNodeProvisionExistingPackageSkipsNetwork(t *testing.T) {
	useTempOwnership(t)
	t.Setenv("VPNUI_OFFLINE", "0")
	dir := t.TempDir()
	// A fake dpkg query confirms the package is already installed.
	if err := os.WriteFile(filepath.Join(dir, "dpkg-query"), []byte("#!/bin/sh\nprintf 'install ok installed'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	marker := filepath.Join(dir, "unexpected-command")
	pm := &packageManager{name: "apt", refresh: []string{"/usr/bin/touch", marker}, install: []string{"/usr/bin/touch", marker}}
	if _, err := pm.installPackage("dkms"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("already installed package still accessed repositories")
	}
}
