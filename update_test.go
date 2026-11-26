package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mtn-man/jfsh/internal/config"
	"github.com/mtn-man/jfsh/internal/jellyfin"
)

const (
	resumeJSON = `{"Items":[{"Id":"r1","Name":"Resume One","Type":"Movie"},{"Id":"r2","Name":"Resume Two","Type":"Movie"}],"TotalRecordCount":2,"StartIndex":0}`
	nextUpJSON = `{"Items":[{"Id":"n1","Name":"Next One","Type":"Movie"}],"TotalRecordCount":1,"StartIndex":0}`
)

// stubServer serves canned Resume and Next Up lists, or a 500 for every request
// while fail is set.
type stubServer struct {
	*httptest.Server
	fail atomic.Bool
}

func newStubServer(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.fail.Load() {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/UserItems/Resume":
			_, _ = w.Write([]byte(resumeJSON))
		case "/Shows/NextUp":
			_, _ = w.Write([]byte(nextUpJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// loadedModel returns a model on the Resume tab whose startup fetch has
// completed against srv, with its last fetch backdated past the focus throttle.
func loadedModel(t *testing.T, srv *stubServer) model {
	t.Helper()
	client, err := jellyfin.NewClient(srv.URL, "test", "device", "test", "token")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	m := initialModel(&config.Session{Client: client, UserID: "user"})
	m = update(t, m, m.initCmd())
	if len(m.items) != 2 {
		t.Fatalf("startup fetch: got %d items, want 2 (err %v)", len(m.items), m.err)
	}
	m.lastFetch = time.Now().Add(-2 * focusRefreshInterval)
	return m
}

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(model)
}

func updateCmd(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	return updated.(model), cmd
}

func itemIDs(items []jellyfin.Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.GetId()
	}
	return ids
}

var nextTabKey = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}

func TestFocusRefreshGuards(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(m *model)
		msg     tea.Msg
		refresh bool
	}{
		{
			name:    "stale list refreshes",
			setup:   func(m *model) {},
			msg:     tea.FocusMsg{},
			refresh: true,
		},
		{
			name:  "recent fetch is throttled",
			setup: func(m *model) { m.lastFetch = time.Now() },
			msg:   tea.FocusMsg{},
		},
		{
			name:  "fetch in flight",
			setup: func(m *model) { m.loading = true },
			msg:   tea.FocusMsg{},
		},
		{
			name: "typing a search",
			setup: func(m *model) {
				m.currentTab = Search
				m.searchInput.Focus()
			},
			msg: tea.FocusMsg{},
		},
		{
			name:  "launching playback",
			setup: func(m *model) { m.playing = &jellyfin.Item{} },
			msg:   tea.FocusMsg{},
		},
		{
			name:  "blur does nothing",
			setup: func(m *model) {},
			msg:   tea.BlurMsg{},
		},
	}
	srv := newStubServer(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loadedModel(t, srv)
			tt.setup(&m)
			gen := m.fetchGen

			got, cmd := updateCmd(t, m, tt.msg)

			if (cmd != nil) != tt.refresh {
				t.Fatalf("cmd returned = %v, want %v", cmd != nil, tt.refresh)
			}
			if tt.refresh {
				if !got.loading {
					t.Error("loading not set after focus refresh")
				}
				if got.fetchGen != gen+1 {
					t.Errorf("fetchGen = %d, want %d", got.fetchGen, gen+1)
				}
			}
		})
	}
}

func TestFocusRefreshKeepsCursorAndFilter(t *testing.T) {
	srv := newStubServer(t)
	m := loadedModel(t, srv)
	m.filterActive = true
	m.filterInput.SetValue("resume")
	m.applyFilter()
	m.currentItem = 1

	m, cmd := updateCmd(t, m, tea.FocusMsg{})
	m = update(t, m, cmd())

	if !m.filterActive || m.filterInput.Value() != "resume" {
		t.Errorf("filter = (%v, %q), want kept", m.filterActive, m.filterInput.Value())
	}
	if id := m.items[m.currentItem].GetId(); id != "r2" {
		t.Errorf("cursor on %q, want r2", id)
	}
}

// A focus reload still in flight when the user switches tabs must not land on
// the new tab, whichever response arrives first.
func TestStaleReloadDoesNotOverwriteTabSwitch(t *testing.T) {
	for _, reloadFirst := range []bool{true, false} {
		name := "next up lands first"
		if reloadFirst {
			name = "stale reload lands first"
		}
		t.Run(name, func(t *testing.T) {
			srv := newStubServer(t)
			m := loadedModel(t, srv)
			m.filterActive = true
			m.filterInput.SetValue("resume")
			m.applyFilter()

			m, reload := updateCmd(t, m, tea.FocusMsg{})
			m, tabSwitch := updateCmd(t, m, nextTabKey)
			if m.currentTab != NextUp {
				t.Fatalf("tab = %v, want NextUp", m.currentTab)
			}
			reloadResult, tabResult := reload(), tabSwitch()

			if reloadFirst {
				m = update(t, m, reloadResult)
				if !m.loading {
					t.Error("stale result cleared loading while the tab switch is still in flight")
				}
				m = update(t, m, tabResult)
			} else {
				m = update(t, m, tabResult)
				m = update(t, m, reloadResult)
			}

			if got := itemIDs(m.items); len(got) != 1 || got[0] != "n1" {
				t.Errorf("items = %v, want [n1]", got)
			}
			if m.loading {
				t.Error("still loading after the latest fetch landed")
			}
			if m.filterActive || m.filterInput.Value() != "" {
				t.Errorf("filter = (%v, %q), want cleared by the tab switch", m.filterActive, m.filterInput.Value())
			}
		})
	}
}

func TestFailedReloadKeepsListAndRecovers(t *testing.T) {
	srv := newStubServer(t)
	m := loadedModel(t, srv)
	m.currentItem = 1

	srv.fail.Store(true)
	m, cmd := updateCmd(t, m, tea.FocusMsg{})
	m = update(t, m, cmd())

	if m.err == nil {
		t.Error("failed reload set no error")
	}
	if got := itemIDs(m.items); len(got) != 2 {
		t.Errorf("items = %v, want the two Resume items kept", got)
	}
	if m.currentItem != 1 {
		t.Errorf("cursor = %d, want 1", m.currentItem)
	}
	if m.loading {
		t.Error("still loading after the failed reload")
	}

	srv.fail.Store(false)
	m.lastFetch = time.Now().Add(-2 * focusRefreshInterval)
	m, cmd = updateCmd(t, m, tea.FocusMsg{})
	m = update(t, m, cmd())

	if m.err != nil {
		t.Errorf("error %v still shown after a successful refresh", m.err)
	}
}

// Unlike a reload, a failed fetch for a new view has nothing valid to keep: the
// old items belong to the previous tab.
func TestFailedTabSwitchClearsList(t *testing.T) {
	srv := newStubServer(t)
	m := loadedModel(t, srv)

	srv.fail.Store(true)
	m, cmd := updateCmd(t, m, nextTabKey)
	m = update(t, m, cmd())

	if m.err == nil {
		t.Error("failed fetch set no error")
	}
	if len(m.items) != 0 {
		t.Errorf("items = %v, want none", itemIDs(m.items))
	}
}

func TestFailedToggleKeepsErrorWithoutReloading(t *testing.T) {
	srv := newStubServer(t)
	m := loadedModel(t, srv)
	m.loading = true

	m, cmd := updateCmd(t, m, toggleWatchedResult{err: errToggle})

	if cmd != nil {
		t.Error("failed toggle started a reload, whose success would clear the error")
	}
	if m.err != errToggle {
		t.Errorf("err = %v, want %v", m.err, errToggle)
	}
	if m.loading {
		t.Error("still loading after the failed toggle")
	}
}

var errToggle = errors.New("toggle failed")

// fakeIINA puts a stand-in iina-cli on PATH that exits immediately, as the real
// one does.
func fakeIINA(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iina"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestPlayingScreenLingers(t *testing.T) {
	fakeIINA(t)
	old := minPlayingScreenTime
	minPlayingScreenTime = 200 * time.Millisecond
	t.Cleanup(func() { minPlayingScreenTime = old })

	srv := newStubServer(t)
	m := loadedModel(t, srv)
	m.session.Host, m.session.Token = srv.URL, "token"

	start := time.Now()
	msg := m.playItem()()
	elapsed := time.Since(start)

	if got, ok := msg.(playbackStopped); !ok || got.err != nil {
		t.Fatalf("got %#v, want playbackStopped with no error", msg)
	}
	if elapsed < minPlayingScreenTime {
		t.Errorf("playbackStopped after %v, want at least %v", elapsed, minPlayingScreenTime)
	}
}

func TestPlayingScreenDoesNotLingerOnError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no iina, so launching fails
	old := minPlayingScreenTime
	minPlayingScreenTime = 2 * time.Second
	t.Cleanup(func() { minPlayingScreenTime = old })

	srv := newStubServer(t)
	m := loadedModel(t, srv)

	start := time.Now()
	msg := m.playItem()()
	elapsed := time.Since(start)

	if got, ok := msg.(playbackStopped); !ok || got.err == nil {
		t.Fatalf("got %#v, want playbackStopped with an error", msg)
	}
	if elapsed >= minPlayingScreenTime {
		t.Errorf("error took %v to surface, want it immediate", elapsed)
	}
}
