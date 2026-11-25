package iina

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mtn-man/jfsh/internal/config"
	"github.com/mtn-man/jfsh/internal/jellyfin"
)

func testItem(id string) jellyfin.Item {
	var item jellyfin.Item
	item.SetId(id)
	item.SetName("Test Item")
	return item
}

// None of these tests run in parallel: they replace PATH with t.Setenv.

func TestPlayIINAMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	session := &config.Session{Host: "http://localhost:8096", Token: "tok"}
	err := Play(session, []jellyfin.Item{testItem("abc")}, 0)
	if err == nil {
		t.Fatal("Play succeeded without iina on PATH")
	}
	if !strings.Contains(err.Error(), "brew install --cask iina") {
		t.Errorf("error %q does not tell the user how to install iina", err)
	}
}

func TestPlayLaunchesIINAWithStreamURL(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	// Stand-in for iina-cli that records its arguments, one per line.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(filepath.Join(dir, player), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	session := &config.Session{Host: "http://localhost:8096/", Token: "tok en"}
	if err := Play(session, []jellyfin.Item{testItem("abc")}, 0); err != nil {
		t.Fatalf("Play: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("fake iina was not run: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{
		"--no-stdin",
		"http://localhost:8096/Videos/abc/stream?static=true&api_key=tok+en",
	}
	if len(got) != len(want) {
		t.Fatalf("iina args = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("iina arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPlayIINAFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, player), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	session := &config.Session{Host: "http://localhost:8096", Token: "tok"}
	if err := Play(session, []jellyfin.Item{testItem("abc")}, 0); err == nil {
		t.Fatal("Play succeeded although iina exited with an error")
	}
}
