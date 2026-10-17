package ui

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func boolPtr(v bool) *bool { return &v }

func slot(panel panelID, height int) gridSlot { return gridSlot{panel: panel, height: height} }

func TestPlanGrid(t *testing.T) {
	hidden := boolPtr(false)
	tests := []struct {
		name   string
		layout config.LayoutConfig
		want   []gridSlot
	}{
		{
			name:   "default layout",
			layout: config.LayoutConfig{},
			want: []gridSlot{
				slot(panelServices, -1), slot(panelResources, -1), slot(panelSearch, 3), slot(panelLogs, -1), slot(panelHelpBar, 1),
			},
		},
		{
			name: "explicit visible flags and heights of 1 are the default",
			layout: config.LayoutConfig{
				Services:  config.ServicesLayout{Height: 1},
				Resources: config.PanelLayout{Visible: boolPtr(true), Height: 1},
				Logs:      config.PanelLayout{Visible: boolPtr(true), Height: 1},
				HelpBar:   config.HelpBarLayout{Visible: boolPtr(true)},
			},
			want: []gridSlot{
				slot(panelServices, -1), slot(panelResources, -1), slot(panelSearch, 3), slot(panelLogs, -1), slot(panelHelpBar, 1),
			},
		},
		{
			name:   "resources hidden",
			layout: config.LayoutConfig{Resources: config.PanelLayout{Visible: hidden}},
			want:   []gridSlot{slot(panelServices, -1), slot(panelSearch, 3), slot(panelLogs, -1), slot(panelHelpBar, 1)},
		},
		{
			name:   "logs hidden",
			layout: config.LayoutConfig{Logs: config.PanelLayout{Visible: hidden}},
			want:   []gridSlot{slot(panelServices, -1), slot(panelResources, -1), slot(panelSearch, 3), slot(panelHelpBar, 1)},
		},
		{
			name:   "help bar hidden",
			layout: config.LayoutConfig{HelpBar: config.HelpBarLayout{Visible: hidden}},
			want:   []gridSlot{slot(panelServices, -1), slot(panelResources, -1), slot(panelSearch, 3), slot(panelLogs, -1)},
		},
		{
			name: "everything optional hidden",
			layout: config.LayoutConfig{
				Resources: config.PanelLayout{Visible: hidden},
				Logs:      config.PanelLayout{Visible: hidden},
				HelpBar:   config.HelpBarLayout{Visible: hidden},
			},
			want: []gridSlot{slot(panelServices, -1), slot(panelSearch, 3)},
		},
		{
			name: "heights are proportional weights",
			layout: config.LayoutConfig{
				Services:  config.ServicesLayout{Height: 3},
				Resources: config.PanelLayout{Height: 2},
				Logs:      config.PanelLayout{Height: 5},
			},
			want: []gridSlot{slot(panelServices, -3), slot(panelResources, -2), slot(panelSearch, 3), slot(panelLogs, -5), slot(panelHelpBar, 1)},
		},
		{
			name: "the height of a hidden panel is ignored",
			layout: config.LayoutConfig{
				Resources: config.PanelLayout{Visible: hidden, Height: 7},
			},
			want: []gridSlot{slot(panelServices, -1), slot(panelSearch, 3), slot(panelLogs, -1), slot(panelHelpBar, 1)},
		},
		{
			name:   "a height that validation would reject still gives a usable grid",
			layout: config.LayoutConfig{Logs: config.PanelLayout{Height: -4}},
			want: []gridSlot{
				slot(panelServices, -1), slot(panelResources, -1), slot(panelSearch, 3), slot(panelLogs, -1), slot(panelHelpBar, 1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planGrid(tt.layout); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("planGrid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The rows the dashboard always had were 0, 0, 3, 0, 1: services, resources,
// search, logs and help bar, with 0 meaning an equal share of what is left.
func TestDefaultPlanHasTheOriginalRows(t *testing.T) {
	var rows []int
	for _, s := range planGrid(config.LayoutConfig{}) {
		if s.height < 0 { // a weight: the original code wrote it as 0
			rows = append(rows, 0)
			continue
		}
		rows = append(rows, s.height)
	}
	if want := []int{0, 0, 3, 0, 1}; !reflect.DeepEqual(rows, want) {
		t.Errorf("default rows = %v, want the original %v", rows, want)
	}
}

// Hidden panels are skipped by Update, so nothing is formatted for them.
func TestUpdateSkipsHiddenPanels(t *testing.T) {
	hidden := boolPtr(false)
	layout := config.LayoutConfig{Resources: config.PanelLayout{Visible: hidden}, Logs: config.PanelLayout{Visible: hidden}}
	d := newTestDashboard(t, Options{Layout: layout})

	d.Update(sampleContainers())

	if got := d.resourcesView.GetText(false); got != "" {
		t.Errorf("hidden resources view was filled: %q", got)
	}
	if got := d.logsView.GetText(false); got != "" {
		t.Errorf("hidden logs view was filled: %q", got)
	}
	if d.servicesTable.GetCell(1, 0).Text != "web" {
		t.Error("the services table, which is always shown, was not updated")
	}

	shown := newTestDashboard(t, Options{})
	shown.Update(sampleContainers())
	if shown.resourcesView.GetText(false) == "" || shown.logsView.GetText(false) == "" {
		t.Error("the visible panels should be filled")
	}
}

// The example configuration spells the built-in layout out; it must produce
// the same grid as no layout at all.
func TestDefaultYAMLLayoutPlansTheDefaultGrid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gowatch.yaml")
	writeTestFile(t, path, string(config.DefaultYAML()))
	cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := planGrid(cfg.Layout), planGrid(config.LayoutConfig{}); !reflect.DeepEqual(got, want) {
		t.Errorf("planGrid(DefaultYAML layout) = %v, want %v", got, want)
	}
	if got, want := visibilityOf(cfg.Layout), visibilityOf(config.LayoutConfig{}); got != want {
		t.Errorf("visibilityOf(DefaultYAML layout) = %+v, want %+v", got, want)
	}
}

// testPanels returns one box per panel, so a grid can be built and measured
// without the dashboard.
func testPanels() [panelCount]tview.Primitive {
	var panels [panelCount]tview.Primitive
	for i := range panels {
		panels[i] = tview.NewBox()
	}
	return panels
}

// rects reads where each panel ended up; a panel that is not in the grid keeps
// the zero rectangle.
func rects(panels [panelCount]tview.Primitive) [panelCount][4]int {
	var out [panelCount][4]int
	for i, p := range panels {
		x, y, w, h := p.GetRect()
		out[i] = [4]int{x, y, w, h}
	}
	return out
}

// The dashboard always used rows 0, 0, 3, 0, 1. The default layout now uses
// -1, -1, 3, -1, 1, which tview must place identically, at every size.
func TestDefaultGridPlacesPanelsLikeTheOriginalRows(t *testing.T) {
	sizes := [][2]int{{80, 24}, {120, 40}, {100, 31}, {200, 61}, {60, 13}, {80, 10}, {80, 8}, {40, 7}, {133, 77}}
	for _, size := range sizes {
		width, height := size[0], size[1]

		legacyPanels := testPanels()
		legacy := tview.NewGrid().SetRows(0, 0, 3, 0, 1).SetColumns(0).
			AddItem(legacyPanels[panelServices], 0, 0, 1, 1, 0, 0, false).
			AddItem(legacyPanels[panelResources], 1, 0, 1, 1, 0, 0, false).
			AddItem(legacyPanels[panelSearch], 2, 0, 1, 1, 0, 0, false).
			AddItem(legacyPanels[panelLogs], 3, 0, 1, 1, 0, 0, true).
			AddItem(legacyPanels[panelHelpBar], 4, 0, 1, 1, 0, 0, false)
		drawPrimitive(t, legacy, width, height)

		panels := testPanels()
		drawPrimitive(t, buildGrid(config.LayoutConfig{}, panels), width, height)

		if got, want := rects(panels), rects(legacyPanels); got != want {
			t.Errorf("%dx%d: panels at %v, the original rows put them at %v", width, height, got, want)
		}
	}
}

func TestBuildGridWeights(t *testing.T) {
	layout := config.LayoutConfig{
		Services:  config.ServicesLayout{Height: 2},
		Resources: config.PanelLayout{Height: 1},
		Logs:      config.PanelLayout{Height: 1},
	}
	panels := testPanels()

	drawPrimitive(t, buildGrid(layout, panels), 80, 40)

	// 40 rows minus the 3 of the search field and the 1 of the help bar leave
	// 36 to share as 2:1:1.
	got := rects(panels)
	want := [panelCount][4]int{
		panelServices:  {0, 0, 80, 18},
		panelResources: {0, 18, 80, 9},
		panelSearch:    {0, 27, 80, 3},
		panelLogs:      {0, 30, 80, 9},
		panelHelpBar:   {0, 39, 80, 1},
	}
	if got != want {
		t.Errorf("panels at %v, want %v", got, want)
	}
}

func TestBuildGridWithHiddenPanels(t *testing.T) {
	hidden := boolPtr(false)
	tests := []struct {
		name      string
		layout    config.LayoutConfig
		wantShown [panelCount]bool
	}{
		{
			name:      "resources hidden",
			layout:    config.LayoutConfig{Resources: config.PanelLayout{Visible: hidden}},
			wantShown: [panelCount]bool{panelServices: true, panelSearch: true, panelLogs: true, panelHelpBar: true},
		},
		{
			name:      "logs hidden",
			layout:    config.LayoutConfig{Logs: config.PanelLayout{Visible: hidden}},
			wantShown: [panelCount]bool{panelServices: true, panelResources: true, panelSearch: true, panelHelpBar: true},
		},
		{
			name:      "help bar hidden",
			layout:    config.LayoutConfig{HelpBar: config.HelpBarLayout{Visible: hidden}},
			wantShown: [panelCount]bool{panelServices: true, panelResources: true, panelSearch: true, panelLogs: true},
		},
		{
			name: "all optional panels hidden",
			layout: config.LayoutConfig{
				Resources: config.PanelLayout{Visible: hidden},
				Logs:      config.PanelLayout{Visible: hidden},
				HelpBar:   config.HelpBarLayout{Visible: hidden},
			},
			wantShown: [panelCount]bool{panelServices: true, panelSearch: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const width, height = 80, 30
			panels := testPanels()
			ux, uy, uw, uh := tview.NewBox().GetRect() // where a box that no grid has placed sits
			untouched := [4]int{ux, uy, uw, uh}

			drawPrimitive(t, buildGrid(tt.layout, panels), width, height)

			// The shown panels fill the screen from top to bottom without gaps.
			nextY := 0
			for id := panelID(0); id < panelCount; id++ {
				x, y, w, h := panels[id].GetRect()
				if !tt.wantShown[id] {
					if [4]int{x, y, w, h} != untouched {
						t.Errorf("hidden panel %d was placed at %d,%d %dx%d, want it left alone", id, x, y, w, h)
					}
					continue
				}
				if y != nextY || w != width || h < 1 {
					t.Errorf("panel %d at %d,%d %dx%d, want it to start at row %d and span the width", id, x, y, w, h, nextY)
				}
				nextY = y + h
			}
			if nextY != height {
				t.Errorf("the shown panels end at row %d, want %d", nextY, height)
			}
			if _, _, _, h := panels[panelSearch].GetRect(); h != searchRowHeight {
				t.Errorf("search field is %d rows high, want %d", h, searchRowHeight)
			}
			if tt.wantShown[panelHelpBar] {
				if _, _, _, h := panels[panelHelpBar].GetRect(); h != helpBarRowHeight {
					t.Errorf("help bar is %d rows high, want %d", h, helpBarRowHeight)
				}
			}
		})
	}
}

// Dashboard-level checks follow: what the user sees when panels are hidden.

func TestDashboardLayoutOnScreen(t *testing.T) {
	hidden := boolPtr(false)
	// Titles that only the named panel draws on its top border, and a text only
	// the help bar contains.
	const (
		servicesMarker  = " Docker Services "
		resourcesMarker = " System Resources "
		searchMarker    = " Filter "
		logsMarker      = " Logs "
		helpMarker      = "Docker View"
	)
	onScreen := func(shot screenshot, marker string) bool {
		if marker == helpMarker {
			_, _, ok := shot.find(marker)
			return ok
		}
		return shot.titleRow(marker) >= 0
	}
	tests := []struct {
		name    string
		layout  config.LayoutConfig
		present []string
		absent  []string
	}{
		{
			name:    "default",
			present: []string{servicesMarker, resourcesMarker, searchMarker, logsMarker, helpMarker},
		},
		{
			name:    "resources hidden",
			layout:  config.LayoutConfig{Resources: config.PanelLayout{Visible: hidden}},
			present: []string{servicesMarker, searchMarker, logsMarker, helpMarker},
			absent:  []string{resourcesMarker},
		},
		{
			name:    "logs hidden",
			layout:  config.LayoutConfig{Logs: config.PanelLayout{Visible: hidden}},
			present: []string{servicesMarker, resourcesMarker, searchMarker, helpMarker},
			absent:  []string{logsMarker},
		},
		{
			name:    "help bar hidden",
			layout:  config.LayoutConfig{HelpBar: config.HelpBarLayout{Visible: hidden}},
			present: []string{servicesMarker, resourcesMarker, searchMarker, logsMarker},
			absent:  []string{helpMarker},
		},
		{
			name: "all optional panels hidden",
			layout: config.LayoutConfig{
				Resources: config.PanelLayout{Visible: hidden},
				Logs:      config.PanelLayout{Visible: hidden},
				HelpBar:   config.HelpBarLayout{Visible: hidden},
			},
			present: []string{servicesMarker, searchMarker},
			absent:  []string{resourcesMarker, logsMarker, helpMarker},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDashboard(t, Options{Layout: tt.layout})
			d.Update(sampleContainers())

			shot := drawDashboard(t, d, 160, 40)

			for _, marker := range tt.present {
				if !onScreen(shot, marker) {
					t.Errorf("%q should be on screen:\n%s", marker, shot.text())
				}
			}
			for _, marker := range tt.absent {
				if onScreen(shot, marker) {
					t.Errorf("%q should not be on screen:\n%s", marker, shot.text())
				}
			}
		})
	}

	t.Run("hidden logs are not drawn even when there are logs", func(t *testing.T) {
		d := newTestDashboard(t, Options{Layout: config.LayoutConfig{Logs: config.PanelLayout{Visible: hidden}}})
		d.Update(sampleContainers())
		if _, _, ok := drawDashboard(t, d, 160, 40).find("listening"); ok {
			t.Error("a log line is on screen although the logs panel is hidden")
		}

		shown := newTestDashboard(t, Options{})
		shown.Update(sampleContainers())
		if _, _, ok := drawDashboard(t, shown, 160, 40).find("listening"); !ok {
			t.Error("the log line should be on screen when the logs panel is visible")
		}
	})

	t.Run("heights change where the panels start", func(t *testing.T) {
		layout := config.LayoutConfig{
			Services:  config.ServicesLayout{Height: 2},
			Resources: config.PanelLayout{Height: 1},
			Logs:      config.PanelLayout{Height: 1},
		}
		d := newTestDashboard(t, Options{Layout: layout})
		d.Update(sampleContainers())

		shot := drawDashboard(t, d, 160, 40)

		wantRows := map[string]int{servicesMarker: 0, resourcesMarker: 18, searchMarker: 27, logsMarker: 30}
		for marker, wantY := range wantRows {
			if y := shot.titleRow(marker); y != wantY {
				t.Errorf("%q is on row %d, want %d", marker, y, wantY)
			}
		}
		if _, y, ok := shot.find(helpMarker); !ok || y != 39 {
			t.Errorf("help bar at row %d (found %v), want the last row", y, ok)
		}
	})
}

func TestDashboardInitialFocus(t *testing.T) {
	tests := []struct {
		want   func(*Dashboard) tview.Primitive
		name   string
		layout config.LayoutConfig
	}{
		{name: "default layout starts in the logs", layout: config.LayoutConfig{}, want: func(d *Dashboard) tview.Primitive { return d.logsView }},
		{name: "hidden logs start in the services table", layout: config.LayoutConfig{Logs: config.PanelLayout{Visible: boolPtr(false)}}, want: func(d *Dashboard) tview.Primitive { return d.servicesTable }},
		{name: "hidden resources keep the logs", layout: config.LayoutConfig{Resources: config.PanelLayout{Visible: boolPtr(false)}}, want: func(d *Dashboard) tview.Primitive { return d.logsView }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDashboard(t, Options{Layout: tt.layout})
			if got := d.app.GetFocus(); got != tt.want(d) {
				t.Errorf("initial focus = %T, want %T", got, tt.want(d))
			}
			if got := d.mainPanel(); got != tt.want(d) {
				t.Errorf("mainPanel() = %T, want %T", got, tt.want(d))
			}

			// The grid itself, when it receives the focus, hands it to the same panel.
			d.app.SetFocus(d.searchField)
			d.app.SetFocus(d.grid)
			if got := d.app.GetFocus(); got != tt.want(d) {
				t.Errorf("focus given to the grid lands on %T, want %T", got, tt.want(d))
			}
		})
	}
}

// With a panel hidden the shortcuts still work and the focus never ends up on
// something that is not on screen.
func TestKeyboardFlowsWithHiddenPanels(t *testing.T) {
	layouts := map[string]config.LayoutConfig{
		"default":           {},
		"logs hidden":       {Logs: config.PanelLayout{Visible: boolPtr(false)}},
		"everything hidden": {Resources: config.PanelLayout{Visible: boolPtr(false)}, Logs: config.PanelLayout{Visible: boolPtr(false)}, HelpBar: config.HelpBarLayout{Visible: boolPtr(false)}},
	}
	press := func(d *Dashboard, key tcell.Key, r rune) {
		event := tcell.NewEventKey(key, r, tcell.ModNone)
		if event = d.handleInput(event); event != nil {
			d.app.GetFocus().InputHandler()(event, func(p tview.Primitive) { d.app.SetFocus(p) })
		}
	}
	for name, layout := range layouts {
		t.Run(name, func(t *testing.T) {
			d := newTestDashboard(t, Options{Layout: layout})
			d.SetupInputCapture()
			home := d.mainPanel()

			t.Run("search with Enter", func(t *testing.T) {
				press(d, tcell.KeyRune, '/')
				if d.app.GetFocus() != d.searchField {
					t.Fatalf("focus after / = %T, want the search field", d.app.GetFocus())
				}
				for _, r := range "web" {
					press(d, tcell.KeyRune, r)
				}
				press(d, tcell.KeyEnter, 0)
				if d.app.GetFocus() != home {
					t.Errorf("focus after Enter = %T, want %T", d.app.GetFocus(), home)
				}
				if d.filterState.SearchText != "web" {
					t.Errorf("search = %q, want %q", d.filterState.SearchText, "web")
				}
			})

			t.Run("search with Esc", func(t *testing.T) {
				press(d, tcell.KeyRune, '/')
				press(d, tcell.KeyEscape, 0)
				if d.app.GetFocus() != home {
					t.Errorf("focus after Esc in the search field = %T, want %T", d.app.GetFocus(), home)
				}
				if d.filterState.SearchText != "" || d.searchField.GetText() != "" {
					t.Errorf("Esc should clear the search, got %q / %q", d.filterState.SearchText, d.searchField.GetText())
				}
			})

			t.Run("Esc outside the search field", func(t *testing.T) {
				d.app.SetFocus(d.searchField)
				d.app.SetFocus(d.servicesTable)
				press(d, tcell.KeyEscape, 0)
				if d.app.GetFocus() != home {
					t.Errorf("focus after Esc = %T, want %T", d.app.GetFocus(), home)
				}
			})

			t.Run("alerts modal", func(t *testing.T) {
				press(d, tcell.KeyRune, 'w')
				if d.app.GetFocus() != d.alertsModal {
					t.Fatalf("focus after w = %T, want the alerts modal", d.app.GetFocus())
				}
				press(d, tcell.KeyRune, 'w')
				if d.app.GetFocus() != home {
					t.Errorf("focus after closing with w = %T, want %T", d.app.GetFocus(), home)
				}

				press(d, tcell.KeyRune, 'w')
				press(d, tcell.KeyEscape, 0)
				if d.app.GetFocus() != home {
					t.Errorf("focus after closing with Esc = %T, want %T", d.app.GetFocus(), home)
				}
			})
		})
	}
}
