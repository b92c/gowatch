package ui

import (
	"reflect"
	"testing"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/filter"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestDashboardHandleInput(t *testing.T) {
	dash := NewDashboard()
	dash.SetupInputCapture()

	t.Run("searchField focused: shortcut keys should not be intercepted", func(t *testing.T) {
		dash.app.SetFocus(dash.searchField)

		shortcutRunes := []rune{'l', 'L', 'a', 'A', 'd', 'D', 'f', '/', 'q'}
		for _, r := range shortcutRunes {
			event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
			res := dash.handleInput(event)
			if res != event {
				t.Errorf("expected key event %q to be passed through when searchField is focused, but got %v", r, res)
			}
		}
	})

	t.Run("logsView focused: shortcut keys should be intercepted", func(t *testing.T) {
		dash.app.SetFocus(dash.logsView)

		lEvent := tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone)
		resL := dash.handleInput(lEvent)
		if resL != nil {
			t.Errorf("expected 'l' event to be intercepted (nil), got %v", resL)
		}

		aEvent := tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone)
		resA := dash.handleInput(aEvent)
		if resA != nil {
			t.Errorf("expected 'a' event to be intercepted (nil), got %v", resA)
		}
		if !dash.awsViewMode {
			t.Errorf("expected awsViewMode to be true after 'a' key press")
		}

		slashEvent := tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone)
		resSlash := dash.handleInput(slashEvent)
		if resSlash != nil {
			t.Errorf("expected '/' event to be intercepted (nil), got %v", resSlash)
		}
		if dash.app.GetFocus() != dash.searchField {
			t.Errorf("expected searchField to be focused after '/' key press")
		}
	})

	t.Run("simulate typing laravel when searchField is focused", func(t *testing.T) {
		dash := NewDashboard()
		dash.SetupInputCapture()
		dash.app.SetFocus(dash.searchField)

		handler := dash.searchField.InputHandler()
		word := "laravel"
		for _, r := range word {
			event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
			ev := dash.handleInput(event)
			if ev != nil {
				handler(ev, func(p tview.Primitive) {
					dash.app.SetFocus(p)
				})
			}
		}
		if dash.searchField.GetText() != "laravel" {
			t.Errorf("expected searchField text to be 'laravel', got %q", dash.searchField.GetText())
		}
	})

	t.Run("simulate pressing / then typing laravel from logsView", func(t *testing.T) {
		dash := NewDashboard()
		dash.SetupInputCapture()
		dash.app.SetFocus(dash.logsView)

		slashEv := tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone)
		ev := dash.handleInput(slashEv)
		if ev != nil {
			dash.logsView.InputHandler()(ev, func(p tview.Primitive) { dash.app.SetFocus(p) })
		}

		if dash.app.GetFocus() != dash.searchField {
			t.Fatalf("expected searchField to be focused after '/', got %v", dash.app.GetFocus())
		}

		word := "laravel"
		for _, r := range word {
			event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
			ev := dash.handleInput(event)
			if ev != nil {
				dash.searchField.InputHandler()(ev, func(p tview.Primitive) {
					dash.app.SetFocus(p)
				})
			}
		}

		if dash.searchField.GetText() != "laravel" {
			t.Errorf("expected searchField text to be 'laravel', got %q", dash.searchField.GetText())
		}
	})
}

func TestApplyInitialFilters(t *testing.T) {
	tests := []struct {
		name       string
		search     string
		wantSearch string
		status     []string
		wantStatus []string
		level      metrics.LogLevel
		wantActive bool
	}{
		{name: "no filters leaves the dashboard untouched"},
		{name: "empty status slice stays off", status: []string{}},
		{name: "search only", search: "api", wantSearch: "api", wantActive: true},
		{name: "search is trimmed like the live search", search: "  api  ", wantSearch: "api", wantActive: true},
		{name: "status only", status: []string{"running", "exited"}, wantStatus: []string{"running", "exited"}, wantActive: true},
		{name: "log level only", level: metrics.LogLevelWarn, wantActive: true},
		{name: "everything", search: "web", status: []string{"running"}, level: metrics.LogLevelError, wantSearch: "web", wantStatus: []string{"running"}, wantActive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dash := NewDashboard()

			dash.ApplyInitialFilters(tt.search, tt.status, tt.level)

			fs := dash.filterState
			if fs.SearchText != tt.wantSearch {
				t.Errorf("SearchText = %q, want %q", fs.SearchText, tt.wantSearch)
			}
			if !reflect.DeepEqual(fs.StatusFilter, tt.wantStatus) {
				t.Errorf("StatusFilter = %#v, want %#v", fs.StatusFilter, tt.wantStatus)
			}
			if fs.MinLogLevel != tt.level {
				t.Errorf("MinLogLevel = %v, want %v", fs.MinLogLevel, tt.level)
			}
			if fs.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v", fs.Active, tt.wantActive)
			}
			if got := dash.searchField.GetText(); got != tt.wantSearch {
				t.Errorf("search field text = %q, want %q", got, tt.wantSearch)
			}
		})
	}

	t.Run("without the call the dashboard starts with no filters", func(t *testing.T) {
		dash := NewDashboard()
		if want := filter.NewFilterState(); !reflect.DeepEqual(dash.filterState, want) {
			t.Errorf("initial filter state = %+v, want %+v", dash.filterState, want)
		}
		if dash.searchField.GetText() != "" {
			t.Errorf("search field = %q, want empty", dash.searchField.GetText())
		}
	})

	t.Run("the status slice is copied", func(t *testing.T) {
		dash := NewDashboard()
		status := []string{"running"}
		dash.ApplyInitialFilters("", status, metrics.LogLevelUnknown)
		status[0] = "exited"
		if dash.filterState.StatusFilter[0] != "running" {
			t.Errorf("StatusFilter = %v, mutating the caller's slice must not change it", dash.filterState.StatusFilter)
		}
	})

	t.Run("the filters are applied to the containers", func(t *testing.T) {
		dash := NewDashboard()
		dash.ApplyInitialFilters("", []string{"running"}, metrics.LogLevelUnknown)
		all := docker.Containers{C: []docker.Container{
			{ID: "aaaaaaaaaaaaaaaa", Service: "web", State: "running"},
			{ID: "bbbbbbbbbbbbbbbb", Service: "db", State: "exited"},
		}}

		got := filter.FilterContainers(all, dash.filterState)

		if len(got.C) != 1 || got.C[0].Service != "web" {
			t.Errorf("filtered containers = %+v, want only the running web service", got.C)
		}
	})

	t.Run("Esc clears filters that came from the config too", func(t *testing.T) {
		dash := NewDashboard()
		dash.SetupInputCapture()
		dash.ApplyInitialFilters("web", []string{"running"}, metrics.LogLevelWarn)
		dash.app.SetFocus(dash.logsView)

		esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
		if res := dash.handleInput(esc); res != nil {
			t.Errorf("Esc should be consumed, got %v", res)
		}

		fs := dash.filterState
		if fs.Active || fs.SearchText != "" || len(fs.StatusFilter) != 0 || fs.MinLogLevel != metrics.LogLevelUnknown {
			t.Errorf("filters after Esc = %+v, want all cleared", fs)
		}
		if dash.searchField.GetText() != "" {
			t.Errorf("search field after Esc = %q, want empty", dash.searchField.GetText())
		}
	})
}
