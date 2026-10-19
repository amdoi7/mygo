package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChecksums(t *testing.T) {
	dist := t.TempDir()
	var artifacts []string
	for _, name := range []string{"App 1.2.0.dmg", "App Setup 1.2.0 amd64.exe", "app-1.2.0-linux-amd64.tar.gz", "update-linux-amd64.json", "App.exe", "linux-amd64/install.sh", "linux-arm64/install.sh"} {
		p := filepath.Join(dist, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("hello"), 0o644)
		artifacts = append(artifacts, p)
	}
	path, err := writeChecksums(dist, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	const hello = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	want := hello + "  App 1.2.0.dmg\n" + hello + "  App Setup 1.2.0 amd64.exe\n" + hello + "  app-1.2.0-linux-amd64.tar.gz\n" + hello + "  install.sh\n"
	if b, _ := os.ReadFile(path); string(b) != want {
		t.Errorf("SHA256SUMS:\n%s\nwant\n%s", b, want)
	}
	if _, err := exec.LookPath("shasum"); err == nil && runtime.GOOS != "windows" {
		cmd := exec.Command("shasum", "-a", "256", "-c", checksumsName)
		cmd.Dir = filepath.Join(dist, "linux-amd64")
		os.WriteFile(filepath.Join(cmd.Dir, checksumsName), []byte(hello+"  install.sh\n"), 0o644)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("shasum -c: %v\n%s", err, out)
		}
	}

	// The lines of an earlier upload stay for the files this one lacks.
	other := strings.Repeat("a", 64)
	if err := mergeChecksums(path, []byte(other+"  App Setup 1.2.0 arm64.exe\r\n"+other+"  install.sh\njunk\n")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), other+"  App Setup 1.2.0 arm64.exe\n") || !strings.Contains(string(b), hello+"  install.sh\n") || strings.Contains(string(b), "junk") {
		t.Errorf("merged SHA256SUMS:\n%s", b)
	}

	if path, err := writeChecksums(dist, nil); path != "" || err != nil {
		t.Errorf("with nothing to publish: %q, %v", path, err)
	}
}

func TestPublishGitHubChecksums(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as gh")
	}
	bin, dist := t.TempDir(), t.TempDir()
	other := strings.Repeat("b", 64)
	// gh: the release exists, with the SHA256SUMS of another platform.
	script := "#!/bin/sh\n[ \"$2\" = download ] || exit 0\nfor a; do last=$a; done\nprintf '" + other + "  App Setup 1.2.0 arm64.exe\\n' > \"$last/SHA256SUMS\"\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	exe := filepath.Join(dist, "App Setup 1.2.0 amd64.exe")
	os.WriteFile(exe, []byte("hello"), 0o644)
	sums, err := writeChecksums(dist, []string{exe})
	if err != nil {
		t.Fatal(err)
	}
	c := &Config{root: t.TempDir(), Version: "1.2.0", Updates: &Updates{GitHub: "me/app", TagPrefix: "v"}}
	if err := publishGitHub(c, []string{exe, sums}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(sums)
	if !strings.Contains(string(b), other+"  App Setup 1.2.0 arm64.exe\n") || !strings.Contains(string(b), "  App Setup 1.2.0 amd64.exe\n") {
		t.Errorf("SHA256SUMS:\n%s", b)
	}
}

func TestInstallPS1(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	key := base64.StdEncoding.EncodeToString(pub)
	for _, tc := range []struct {
		updates Updates
		want    []string
	}{
		{Updates{PublicKey: key, GitHub: "me/my-app", TagPrefix: "v"}, []string{
			"#   irm https://github.com/me/my-app/releases/latest/download/install.ps1 | iex\n",
			"$releases = 'https://github.com/me/my-app/releases/latest/download/'\n",
			"$api = ''\n",
			"$github = $true\n",
			"$assetsApi = 'https://api.github.com/repos/me/my-app/releases/latest'\n",
		}},
		{Updates{PublicKey: key, GitHub: "me/my-app", TagPrefix: "desktop-v"}, []string{
			"#   irm https://github.com/me/my-app/releases/download/desktop-v1.2.0/install.ps1 | iex\n",
			"$releases = 'https://github.com/me/my-app/releases/download/'\n",
			"$api = 'https://api.github.com/repos/me/my-app/releases?per_page=100'\n",
			"$tagPrefix = 'desktop-v'\n",
			"$assetsApi = 'https://api.github.com/repos/me/my-app/releases/tags/'\n",
		}},
		{Updates{PublicKey: key, URL: "https://dl.example.com/my-app"}, []string{
			"$releases = 'https://dl.example.com/my-app/'\n",
			"$github = $false\n",
		}},
	} {
		c := &Config{Name: "Jane's App", Identifier: "com.example.app", Version: "1.2.0", Updates: &tc.updates}
		script := installPS1(c)
		for _, line := range append(tc.want,
			"$appName = 'Jane''s App'\n",
			"$exeName = 'Jane''s App.exe'\n",
			"$setupPrefix = 'Jane''s App Setup '\n",
			"$identifier = 'com.example.app'\n",
			"$downloadUrl = $env:JANE_S_APP_DOWNLOAD_URL\n",
		) {
			if !strings.Contains(script, line) {
				t.Errorf("install.ps1 of %+v has no %q", tc.updates, line)
			}
		}
		// The installer it looks for is the one the build names.
		if !strings.HasPrefix(installerFileName(c, "arm64"), "Jane's App Setup ") {
			t.Errorf("installer %s", installerFileName(c, "arm64"))
		}
		if pwsh, err := exec.LookPath("pwsh"); err == nil {
			path := filepath.Join(t.TempDir(), "install.ps1")
			os.WriteFile(path, []byte(script), 0o644)
			if out, err := exec.Command(pwsh, "-NoProfile", "-Command", "$null = [scriptblock]::Create((Get-Content -Raw '"+path+"'))").CombinedOutput(); err != nil {
				t.Errorf("pwsh cannot parse install.ps1: %v\n%s", err, out)
			}
		}
	}
	if got := envName("my-app"); got != "MY_APP" {
		t.Errorf("envName(my-app) = %s", got)
	}
	if got := envName("2do"); got != "APP_2DO" {
		t.Errorf("envName(2do) = %s", got)
	}
}
