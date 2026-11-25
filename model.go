package main

import (
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mtn-man/jfsh/internal/config"
	"github.com/mtn-man/jfsh/internal/jellyfin"
)

type tab int

const (
	Resume tab = iota
	NextUp
	RecentlyAdded
	Search
	ResumeTabName        = "Resume"
	NextUpTabName        = "Next Up"
	RecentlyAddedTabName = "Recently Added"
	SearchTabName        = "Search"
)

// focusRefreshInterval is the minimum time since the last fetch before regaining
// terminal focus triggers another one. Focus also changes on Ghostty tab and split
// switches, so without this an ordinary alt-tab would hit the server every time.
const focusRefreshInterval = 15 * time.Second

type model struct {
	keyMap KeyMap
	help   help.Model

	width  int
	height int

	session *config.Session

	currentTab  tab
	searchInput textinput.Model

	items       []jellyfin.Item
	allItems    []jellyfin.Item
	currentItem int

	// fetchGen numbers fetches. Each fetchItemsResult carries the number it was
	// issued under, and only the latest one is applied.
	fetchGen int

	filterActive bool
	filterInput  textinput.Model

	currentSeries *jellyfin.Item

	playing *jellyfin.Item

	err     error
	spinner spinner.Model
	loading bool

	// lastFetch is when the last fetch was issued. It throttles refreshes on focus.
	lastFetch time.Time

	// initCmd is the startup fetch, built in initialModel rather than in Init.
	// Init has a value receiver, so the fetchGen and lastFetch that fetchItems
	// records there would be lost, and the first result dropped as stale.
	initCmd tea.Cmd
}

func initialModel(session *config.Session) model {
	searchInput := textinput.New()
	searchInput.Prompt = "Search: "
	searchInput.Width = 40

	filterInput := textinput.New()
	filterInput.Prompt = "Filter: "
	filterInput.Width = 40

	m := model{
		keyMap:      defaultKeyMap(),
		help:        help.New(),
		session:     session,
		searchInput: searchInput,
		filterInput: filterInput,
		spinner:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.initCmd = m.fetchItems()
	m.updateKeys()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.initCmd,
		m.spinner.Tick,
	)
}
