package main

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mtn-man/jfsh/internal/iina"
	"github.com/mtn-man/jfsh/internal/jellyfin"
)

type playbackStopped struct {
	err error
}

func (m *model) playItem() tea.Cmd {
	session := m.session
	item := m.items[m.currentItem]

	launch := func() error {
		if !jellyfin.IsEpisode(item) {
			return iina.Play(session, []jellyfin.Item{item}, 0)
		}
		// get all episodes of the series and find the index of selected episode
		items, err := session.Client.GetEpisodes(item)
		if err != nil {
			return err
		}
		idx := slices.IndexFunc(items, func(i jellyfin.Item) bool {
			return item.GetId() == i.GetId()
		})
		idx = max(0, idx) // sanity check
		return iina.Play(session, items, idx)
	}

	return func() tea.Msg {
		start := time.Now()
		// playbackStopped, not a bare error: the error case doesn't clear
		// m.playing, which would leave every key disabled, ctrl+c included.
		if err := launch(); err != nil {
			// No wait on failure: the error should show right away.
			return playbackStopped{err}
		}
		// Hold the "Playing" screen long enough to be read.
		time.Sleep(minPlayingScreenTime - time.Since(start))
		return playbackStopped{nil}
	}
}

type toggleWatchedResult struct {
	err error
}

func (m *model) toggleWatchedStatus() tea.Cmd {
	m.loading = true
	session := m.session
	item := m.items[m.currentItem]
	if jellyfin.Watched(item) {
		return func() tea.Msg {
			if err := session.Client.MarkAsUnwatched(item); err != nil {
				return toggleWatchedResult{err}
			}
			return toggleWatchedResult{nil}
		}
	} else {
		return func() tea.Msg {
			if err := session.Client.MarkAsWatched(item); err != nil {
				return toggleWatchedResult{err}
			}
			return toggleWatchedResult{nil}
		}
	}
}

type fetchItemsResult struct {
	items []jellyfin.Item
	err   error

	// gen is the fetchGen the request was issued under. Update ignores any result
	// but the latest, so a slow reload can't land on top of a tab switch made
	// while it was in flight.
	gen int

	// reload is set by reloadItems: keep the filter, and put the cursor back on
	// keepID. It travels with the request rather than sitting on the model, so a
	// superseded reload can't leak it into the fetch that replaced it.
	reload bool
	keepID string
}

// fetchItems loads the current view from scratch, clearing the filter and
// cursor, which is what a tab change or drilling into a series wants.
func (m *model) fetchItems() tea.Cmd {
	return m.startFetch(false, "")
}

// reloadItems refetches the current list without disturbing the view: the filter
// stays applied and the cursor returns to the item it was on.
func (m *model) reloadItems() tea.Cmd {
	keepID := ""
	if m.currentItem < len(m.items) {
		keepID = m.items[m.currentItem].GetId()
	}
	return m.startFetch(true, keepID)
}

func (m *model) startFetch(reload bool, keepID string) tea.Cmd {
	m.loading = true
	m.lastFetch = time.Now()
	m.fetchGen++
	gen := m.fetchGen
	query := m.itemQuery()
	return func() tea.Msg {
		items, err := query()
		return fetchItemsResult{items: items, err: err, gen: gen, reload: reload, keepID: keepID}
	}
}

// itemQuery returns the request for the current view. It copies what it needs
// out of the model now, so the request reads nothing from the model when it
// later runs on another goroutine.
func (m *model) itemQuery() func() ([]jellyfin.Item, error) {
	client := m.session.Client
	if m.currentSeries != nil {
		series := *m.currentSeries
		return func() ([]jellyfin.Item, error) { return client.GetEpisodes(series) }
	}
	switch m.currentTab {
	case Resume:
		userID := m.session.UserID
		return func() ([]jellyfin.Item, error) { return client.GetResume(userID) }
	case NextUp:
		return client.GetNextUp
	case RecentlyAdded:
		return client.GetRecentlyAdded
	case Search:
		query := m.searchInput.Value()
		return func() ([]jellyfin.Item, error) {
			if query == "" {
				return nil, nil
			}
			return client.Search(query)
		}
	default:
		panic("oops, selected tab is not in switch statement")
	}
}

// restoreSelection puts the cursor back on the item with id. Resume and Next Up
// reorder as things are watched, so holding the old index would silently leave
// the cursor on a different item. If the item is gone, applyFilter has already
// clamped the index.
func (m *model) restoreSelection(id string) {
	if id == "" {
		return
	}
	for i, item := range m.items {
		if item.GetId() == id {
			m.currentItem = i
			return
		}
	}
}

// shouldRefreshOnFocus reports whether regaining terminal focus should reload the
// list. It skips the reload while a fetch is already in flight, since that fetch
// will bring the list up to date anyway. It also skips while IINA is launching,
// while the user is typing a search (the fetch would run the half-typed query), and
// within focusRefreshInterval of the last fetch.
func (m *model) shouldRefreshOnFocus() bool {
	switch {
	case m.loading, m.playing != nil, m.searchInput.Focused():
		return false
	}
	return time.Since(m.lastFetch) >= focusRefreshInterval
}

func (m *model) applyFilter() {
	if !m.filterActive || m.filterInput.Value() == "" {
		m.items = m.allItems
		if m.currentItem >= len(m.items) {
			m.currentItem = 0
		}
		return
	}

	filterText := strings.ToLower(m.filterInput.Value())
	var filtered []jellyfin.Item

	for _, item := range m.allItems {
		title := strings.ToLower(jellyfin.GetItemTitle(item))
		desc := strings.ToLower(jellyfin.GetItemDescription(item))

		if strings.Contains(title, filterText) || strings.Contains(desc, filterText) {
			filtered = append(filtered, item)
		}
	}

	m.items = filtered
	if m.currentItem >= len(m.items) {
		m.currentItem = 0
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case error:
		m.err = msg
		return m, nil

	case playbackStopped:
		if msg.err != nil {
			m.err = msg.err
		}
		m.playing = nil
		m.updateKeys()
		return m, nil

	case toggleWatchedResult:
		m.loading = false
		if msg.err != nil {
			// No reload: nothing changed, and its success would clear the error
			// before it could be read.
			m.err = msg.err
			return m, nil
		}
		return m, m.reloadItems()

	case fetchItemsResult:
		if msg.gen != m.fetchGen {
			// Superseded. The newer fetch is still in flight, so stay loading.
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			if msg.reload {
				// Keep the list being refreshed rather than blanking it. A failed
				// background refresh (e.g. no network right after waking) would
				// otherwise empty the view on an ordinary alt-tab.
				return m, nil
			}
			msg.items = nil
		} else {
			m.err = nil
		}
		m.allItems = msg.items
		if msg.reload {
			m.applyFilter()
			m.restoreSelection(msg.keepID)
		} else {
			m.filterInput.SetValue("")
			m.filterActive = false
			m.applyFilter()
		}
		m.updateKeys()
		return m, nil

	case tea.FocusMsg:
		if !m.shouldRefreshOnFocus() {
			return m, nil
		}
		slog.Debug("refreshing on focus", "since_last_fetch", time.Since(m.lastFetch).Round(time.Second).String())
		return m, m.reloadItems()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if key.Matches(msg, m.keyMap.ForceQuit) {
			return m, tea.Quit
		}

		if m.searchInput.Focused() {
			switch {
			case key.Matches(msg, m.keyMap.CancelWhileSearching):
				m.searchInput.Blur()
				m.updateKeys()
				return m, nil
			case key.Matches(msg, m.keyMap.AcceptWhileSearching):
				m.searchInput.Blur()
				m.updateKeys()
				return m, m.fetchItems()
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}

		if m.filterInput.Focused() {
			switch {
			case key.Matches(msg, m.keyMap.CancelWhileFiltering):
				m.filterInput.Blur()
				m.filterInput.SetValue("")
				m.filterActive = false
				m.applyFilter()
				m.updateKeys()
				return m, nil
			case key.Matches(msg, m.keyMap.AcceptWhileFiltering):
				m.filterInput.Blur()
				m.updateKeys()
				return m, nil
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.applyFilter()
			return m, cmd
		}

		switch {
		case key.Matches(msg, m.keyMap.CursorUp):
			if m.currentItem > 0 {
				m.currentItem--
			}
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.CursorDown):
			if m.currentItem < len(m.items)-1 {
				m.currentItem++
			}
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.PageUp):
			jump := m.height / 5
			m.currentItem = max(0, m.currentItem-jump)
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.PageDown):
			jump := m.height / 5
			m.currentItem = min(len(m.items)-1, m.currentItem+jump)
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.GoToEnd):
			m.currentItem = len(m.items) - 1
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.GoToStart):
			m.currentItem = 0
			m.updateKeys()
			return m, nil

		case key.Matches(msg, m.keyMap.NextTab):
			if m.currentTab < Search {
				m.currentTab++
			} else {
				m.currentTab = 0
			}
			m.updateKeys()
			return m, m.fetchItems()
		case key.Matches(msg, m.keyMap.PrevTab):
			if m.currentTab > 0 {
				m.currentTab--
			} else {
				m.currentTab = Search
			}
			m.updateKeys()
			return m, m.fetchItems()

		case key.Matches(msg, m.keyMap.Search):
			m.searchInput.Focus()
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.ClearSearch):
			m.searchInput.SetValue("")
			m.updateKeys()
			return m, m.fetchItems()

		case key.Matches(msg, m.keyMap.Filter):
			m.filterActive = true
			m.filterInput.Focus()
			m.updateKeys()
			return m, nil
		case key.Matches(msg, m.keyMap.ClearFilter):
			m.filterInput.SetValue("")
			m.filterActive = false
			m.applyFilter()
			m.updateKeys()
			return m, nil

		case key.Matches(msg, m.keyMap.Select):
			item := m.items[m.currentItem]
			if jellyfin.IsSeries(item) {
				m.currentSeries = &item
				m.updateKeys()
				return m, m.fetchItems()
			}
			m.playing = &item
			m.updateKeys()
			return m, m.playItem()

		case key.Matches(msg, m.keyMap.Back):
			m.currentSeries = nil
			m.updateKeys()
			return m, m.fetchItems()

		case key.Matches(msg, m.keyMap.ShowFullHelp):
			m.help.ShowAll = !m.help.ShowAll
			m.updateKeys()
			return m, nil

		case key.Matches(msg, m.keyMap.CloseFullHelp):
			m.help.ShowAll = !m.help.ShowAll
			m.updateKeys()
			return m, nil

		case key.Matches(msg, m.keyMap.ToggleWatched):
			return m, m.toggleWatchedStatus()

		case key.Matches(msg, m.keyMap.Refresh):
			return m, m.reloadItems()

		case key.Matches(msg, m.keyMap.Quit):
			return m, tea.Quit
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}
