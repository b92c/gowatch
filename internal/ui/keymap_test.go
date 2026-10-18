package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
)

func runeEvent(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }
func keyEvent(k tcell.Key) *tcell.EventKey {
	mod := tcell.ModNone
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		mod = tcell.ModCtrl
	}
	return tcell.NewEventKey(k, 0, mod)
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		input string
		want  KeyCombo
	}{
		{"q", runeCombo('q')},
		{"Q", runeCombo('Q')},
		{"G", runeCombo('G')},
		{"/", runeCombo('/')},
		{"?", runeCombo('?')},
		{"[", runeCombo('[')},
		{"]", runeCombo(']')},
		{"5", runeCombo('5')},
		{"é", runeCombo('é')},
		{" ", runeCombo(' ')},
		{" q ", runeCombo('q')},
		{"space", runeCombo(' ')},
		{"SPACE", runeCombo(' ')},
		{"ctrl+d", KeyCombo{Key: tcell.KeyCtrlD}},
		{"Ctrl+D", KeyCombo{Key: tcell.KeyCtrlD}},
		{"CTRL+u", KeyCombo{Key: tcell.KeyCtrlU}},
		{"ctrl+a", KeyCombo{Key: tcell.KeyCtrlA}},
		{"ctrl+z", KeyCombo{Key: tcell.KeyCtrlZ}},
		{"ctrl+c", KeyCombo{Key: tcell.KeyCtrlC}}, // readable here; BuildKeymap refuses it
		{"ctrl+j", KeyCombo{Key: tcell.KeyCtrlJ}},
		{"esc", KeyCombo{Key: tcell.KeyEscape}},
		{"Esc", KeyCombo{Key: tcell.KeyEscape}},
		{"escape", KeyCombo{Key: tcell.KeyEscape}},
		{"ESCAPE", KeyCombo{Key: tcell.KeyEscape}},
		{"enter", KeyCombo{Key: tcell.KeyEnter}},
		{"Enter", KeyCombo{Key: tcell.KeyEnter}},
		{"tab", KeyCombo{Key: tcell.KeyTab}},
		{"up", KeyCombo{Key: tcell.KeyUp}},
		{"Down", KeyCombo{Key: tcell.KeyDown}},
		{"pgup", KeyCombo{Key: tcell.KeyPgUp}},
		{"PgDn", KeyCombo{Key: tcell.KeyPgDn}},
		{"home", KeyCombo{Key: tcell.KeyHome}},
		{"End", KeyCombo{Key: tcell.KeyEnd}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseKey(tt.input)
			if err != nil {
				t.Fatalf("ParseKey(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseKey(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}

	invalid := []struct {
		input string
		want  string
	}{
		{"", "empty key"},
		{"   ", "empty key"},
		{"qq", "unknown key"},
		{"ctrl", "unknown key"},
		{"ctrl+", "unknown key"},
		{"ctrl+1", "unknown key"},
		{"ctrl+dd", "unknown key"},
		{"ctrl+shift+d", "unknown key"},
		{"alt+d", "unknown key"},
		{"shift+tab", "unknown key"},
		{"f1", "unknown key"},
		{"left", "unknown key"},
		{"backspace", "unknown key"},
		{"pageup", "unknown key"},
		{"ctrl+h", "Backspace"},
		{"ctrl+i", "Tab"},
		{"ctrl+m", "Enter"},
		{"\t", "control character"},
		{"\x1b", "control character"},
		{"\x7f", "control character"},
	}
	for _, tt := range invalid {
		t.Run("invalid "+tt.input, func(t *testing.T) {
			got, err := ParseKey(tt.input)
			if err == nil {
				t.Fatalf("ParseKey(%q) = %+v, want an error", tt.input, got)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseKey(%q) error = %q, want it to contain %q", tt.input, err, tt.want)
			}
			if strings.Contains(err.Error(), "unknown key") && !strings.Contains(err.Error(), "valid keys:") {
				t.Errorf("error %q does not say which keys are valid", err)
			}
		})
	}
}

// The spelling KeyCombo.String gives is the one ParseKey reads, and the label is
// what the help bar and the help page print.
func TestKeyComboSpellingAndLabel(t *testing.T) {
	tests := []struct {
		spelling string
		label    string
	}{
		{"q", "q"}, {"G", "G"}, {"/", "/"}, {"?", "?"}, {"[", "["}, {"space", "Space"},
		{"esc", "Esc"}, {"enter", "Enter"}, {"tab", "Tab"},
		{"up", "↑"}, {"down", "↓"}, {"pgup", "PgUp"}, {"pgdn", "PgDn"}, {"home", "Home"}, {"end", "End"},
		{"ctrl+d", "Ctrl+D"}, {"ctrl+u", "Ctrl+U"}, {"ctrl+a", "Ctrl+A"}, {"ctrl+z", "Ctrl+Z"},
	}
	for _, tt := range tests {
		t.Run(tt.spelling, func(t *testing.T) {
			k, err := ParseKey(tt.spelling)
			if err != nil {
				t.Fatal(err)
			}
			if got := k.String(); got != tt.spelling {
				t.Errorf("String() = %q, want %q", got, tt.spelling)
			}
			if got := k.Label(); got != tt.label {
				t.Errorf("Label() = %q, want %q", got, tt.label)
			}
		})
	}
}

// A parsed key is found by the very event a terminal produces for it.
func TestParsedKeysMatchTheirEvents(t *testing.T) {
	tests := []struct {
		event    *tcell.EventKey
		spelling string
	}{
		{runeEvent('q'), "q"},
		{runeEvent('G'), "G"},
		{runeEvent('?'), "?"},
		{runeEvent(' '), "space"},
		{runeEvent('é'), "é"},
		{tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModAlt), "q"}, // modifiers are not part of a key
		{tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), "esc"},
		{tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), "enter"},
		{tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), "tab"},
		{tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), "up"},
		{tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), "down"},
		{tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone), "pgup"},
		{tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone), "pgdn"},
		{tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), "home"},
		{tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), "end"},
		{tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl), "ctrl+d"},
		{tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModCtrl), "ctrl+u"},
		// What a terminal sends for Ctrl+D: NewEventKey turns the control
		// character into the same key.
		{tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModCtrl), "ctrl+d"},
	}
	for _, tt := range tests {
		k, err := ParseKey(tt.spelling)
		if err != nil {
			t.Fatalf("ParseKey(%q): %v", tt.spelling, err)
		}
		if got := comboOf(tt.event); got != k {
			t.Errorf("event key %v rune %q = %+v, want %+v (%q)", tt.event.Key(), tt.event.Rune(), got, k, tt.spelling)
		}
	}

	// The terminal's own spellings of the three aliased keys are the named keys.
	for _, tt := range []struct {
		event    *tcell.EventKey
		spelling string
	}{
		{tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone), "ctrl+h"},
		{tcell.NewEventKey(tcell.KeyRune, '\t', tcell.ModNone), "ctrl+i"},
		{tcell.NewEventKey(tcell.KeyRune, '\r', tcell.ModNone), "ctrl+m"},
	} {
		if _, err := ParseKey(tt.spelling); err == nil {
			t.Errorf("ParseKey(%q) should be refused: the terminal never sends it", tt.spelling)
		}
		if _, ok := DefaultKeymap().Lookup(tt.event); ok {
			t.Errorf("the default keymap binds the event of %s", tt.spelling)
		}
	}
}

// legacyKeyTable is the keymap GoWatch had before key bindings were
// configurable, written out as keys to actions: what handleInput did with each
// key, copied from commit 155d4ff. The default preset has to be exactly this,
// plus ? for the help page, e and E for the log export and the scroll keys the panels handle themselves.
var legacyKeyTable = map[string]Action{
	"q": ActionQuit,
	"/": ActionSearch, "f": ActionSearch,
	"l": ActionCycleLogLevel, "L": ActionCycleLogLevel,
	"d": ActionDockerView, "D": ActionDockerView,
	"a": ActionAWSView, "A": ActionAWSView,
	"w": ActionToggleAlerts, "W": ActionToggleAlerts,
	"c": ActionClearResolvedAlerts, "C": ActionClearResolvedAlerts,
	"esc": ActionClearFilter,
}

// nativeKeyTable is what the default preset lists for the scroll actions. The
// panels handle these keys themselves; the actions only describe them in the
// help page and the help bar.
var nativeKeyTable = map[string]Action{
	"up": ActionScrollUp, "down": ActionScrollDown, "pgup": ActionPageUp, "pgdn": ActionPageDown,
	"home": ActionScrollTop, "end": ActionScrollBottom,
}

func TestDefaultPresetIsTheOriginalKeymap(t *testing.T) {
	km := DefaultKeymap()

	want := map[string]Action{"?": ActionHelp, "e": ActionExportLogs, "E": ActionExportLogs}
	for key, action := range legacyKeyTable {
		want[key] = action
	}
	for key, action := range nativeKeyTable {
		want[key] = action
	}

	for spelling, action := range want {
		k, err := ParseKey(spelling)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := km.lookup[k]
		if !ok || got != action {
			t.Errorf("key %q = %q (bound %v), want %q", spelling, got, ok, action)
		}
	}
	if len(km.lookup) != len(want) {
		var extra []string
		for k := range km.lookup {
			if _, ok := want[k.String()]; !ok {
				extra = append(extra, k.String())
			}
		}
		t.Errorf("the default preset binds %d keys, want %d; unexpected: %v", len(km.lookup), len(want), extra)
	}

	// Lower and upper case are different keys.
	for _, r := range "qfdawcl" {
		if _, ok := km.Lookup(runeEvent(r)); !ok {
			t.Errorf("%q is not bound", r)
		}
	}
	for _, r := range "QF" { // the original bound no capital Q or F
		if action, ok := km.Lookup(runeEvent(r)); ok {
			t.Errorf("%q is bound to %q, but the original ignored it", r, action)
		}
	}
}

func TestDefaultPresetKeyOrder(t *testing.T) {
	// The help bar shows the first key of an action and offers the second key of
	// search as the filter key, so the order is part of the preset.
	km := DefaultKeymap()
	for action, want := range map[Action][]string{
		ActionSearch:        {"/", "f"},
		ActionCycleLogLevel: {"l", "L"},
		ActionToggleAlerts:  {"w", "W"},
		ActionDockerView:    {"d", "D"},
		ActionAWSView:       {"a", "A"},
		ActionQuit:          {"q"},
		ActionHelp:          {"?"},
	} {
		var got []string
		for _, k := range km.Keys(action) {
			got = append(got, k.String())
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys = %v, want %v", action, got, want)
		}
	}
}

func TestKeymapLookupIgnoresModifiers(t *testing.T) {
	km := DefaultKeymap()
	// Alt+q quit the dashboard before key bindings existed, and Esc did whatever
	// modifier came with it.
	for _, ev := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModAlt),
		tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModMeta),
	} {
		if action, ok := km.Lookup(ev); !ok || action != ActionQuit {
			t.Errorf("Lookup(%v with modifiers %v) = %q, %v, want quit", ev.Rune(), ev.Modifiers(), action, ok)
		}
	}
	for _, mod := range []tcell.ModMask{tcell.ModShift, tcell.ModCtrl, tcell.ModAlt} {
		if action, ok := km.Lookup(tcell.NewEventKey(tcell.KeyEscape, 0, mod)); !ok || action != ActionClearFilter {
			t.Errorf("Esc with modifiers %v = %q, %v, want clear_filter", mod, action, ok)
		}
	}
}

func TestVimPreset(t *testing.T) {
	km, err := BuildKeymap(config.KeybindingsConfig{Preset: "vim"})
	if err != nil {
		t.Fatalf("BuildKeymap(vim) error = %v", err)
	}
	def := DefaultKeymap()

	t.Run("adds j k g G ctrl+d ctrl+u", func(t *testing.T) {
		tests := []struct {
			event *tcell.EventKey
			want  Action
		}{
			{runeEvent('j'), ActionScrollDown},
			{runeEvent('k'), ActionScrollUp},
			{runeEvent('g'), ActionScrollTop},
			{runeEvent('G'), ActionScrollBottom},
			{keyEvent(tcell.KeyCtrlD), ActionPageDown},
			{keyEvent(tcell.KeyCtrlU), ActionPageUp},
		}
		for _, tt := range tests {
			if got, ok := km.Lookup(tt.event); !ok || got != tt.want {
				t.Errorf("vim: %v %q = %q (bound %v), want %q", tt.event.Key(), tt.event.Rune(), got, ok, tt.want)
			}
			if _, ok := def.Lookup(tt.event); ok {
				t.Errorf("the default preset binds %v %q, which only vim should add", tt.event.Key(), tt.event.Rune())
			}
		}
	})

	t.Run("keeps every default key", func(t *testing.T) {
		for k, action := range def.lookup {
			if got, ok := km.lookup[k]; !ok || got != action {
				t.Errorf("key %q = %q in vim (bound %v), want the default's %q", k.String(), got, ok, action)
			}
		}
	})

	t.Run("adds nothing else and the first keys stay the default's", func(t *testing.T) {
		if got, want := len(km.lookup), len(def.lookup)+6; got != want {
			t.Errorf("vim binds %d keys, want %d", got, want)
		}
		for _, action := range Actions() {
			if km.Keys(action)[0] != def.Keys(action)[0] {
				t.Errorf("%s: the first key is %v in vim, %v in the default preset", action, km.Keys(action)[0], def.Keys(action)[0])
			}
		}
	})
}

func TestPresetsBindEveryActionWithoutConflicts(t *testing.T) {
	for _, name := range PresetNames() {
		t.Run(name, func(t *testing.T) {
			km, err := BuildKeymap(config.KeybindingsConfig{Preset: name})
			if err != nil {
				t.Fatalf("BuildKeymap(%q) error = %v", name, err)
			}
			for _, action := range Actions() {
				if len(km.Keys(action)) == 0 {
					t.Errorf("preset %q leaves %s without a key", name, action)
				}
			}
			total := 0
			for _, action := range Actions() {
				total += len(km.Keys(action))
			}
			if total != len(km.lookup) {
				t.Errorf("preset %q lists %d keys but binds %d: a key is on two actions", name, total, len(km.lookup))
			}
		})
	}
	if got, want := PresetNames(), []string{"default", "vim"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PresetNames() = %v, want %v (there is no emacs preset)", got, want)
	}
}

func TestPresetsDoNotUseReservedOrAliasedKeys(t *testing.T) {
	for name, preset := range presets {
		for action, keys := range preset {
			for _, k := range keys {
				if k == reservedKey {
					t.Errorf("preset %s binds Ctrl+C to %s", name, action)
				}
				if native, ok := scrollKeys[k.Key]; ok && native != action {
					t.Errorf("preset %s binds the scroll key %s to %s", name, k, action)
				}
			}
		}
	}
}

func TestActionMetadata(t *testing.T) {
	want := []string{
		"help", "quit", "docker_view", "aws_view", "search", "cycle_log_level", "export_logs", "clear_filter",
		"toggle_alerts", "clear_resolved_alerts", "scroll_up", "scroll_down", "page_up", "page_down",
		"scroll_top", "scroll_bottom",
	}
	if got := ActionNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("ActionNames() = %v, want %v", got, want)
	}

	known := map[string]bool{}
	for _, group := range groupOrder {
		known[group] = true
	}
	for _, action := range Actions() {
		if action.Description() == "" {
			t.Errorf("%s has no description", action)
		}
		if !known[action.Group()] {
			t.Errorf("%s is in the group %q, which the help page does not list", action, action.Group())
		}
		if string(action) != strings.ToLower(string(action)) || strings.ContainsAny(string(action), " -") {
			t.Errorf("action name %q is not snake_case", action)
		}
	}
	for _, group := range groupOrder {
		found := false
		for _, action := range Actions() {
			found = found || action.Group() == group
		}
		if !found {
			t.Errorf("group %q has no action", group)
		}
	}
	if Action("nope").Group() != "" || Action("nope").Description() != "" {
		t.Error("an unknown action should have no group or description")
	}
}

func TestBuildKeymap(t *testing.T) {
	keys := func(km Keymap, a Action) []string {
		var out []string
		for _, k := range km.Keys(a) {
			out = append(out, k.String())
		}
		return out
	}

	t.Run("an empty config is the default preset", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(km, DefaultKeymap()) {
			t.Error("BuildKeymap of an empty config differs from DefaultKeymap()")
		}
	})

	t.Run("preset names are case-insensitive", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Preset: "  VIM "})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := km.Lookup(runeEvent('j')); !ok {
			t.Error("j is not bound in the vim preset")
		}
	})

	t.Run("an override replaces all the keys of its action", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(km, ActionQuit); !reflect.DeepEqual(got, []string{"x"}) {
			t.Errorf("quit keys = %v, want [x]", got)
		}
		if _, ok := km.Lookup(runeEvent('q')); ok {
			t.Error("q still runs an action after quit was overridden")
		}
		if action, ok := km.Lookup(runeEvent('x')); !ok || action != ActionQuit {
			t.Errorf("x = %q, %v, want quit", action, ok)
		}
		// Everything else is untouched.
		for _, a := range Actions() {
			if a != ActionQuit && !reflect.DeepEqual(km.Keys(a), DefaultKeymap().Keys(a)) {
				t.Errorf("%s changed to %v", a, keys(km, a))
			}
		}
	})

	t.Run("an override on top of the vim preset replaces the vim keys too", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Preset: "vim", Bindings: map[string][]string{"scroll_down": {"n"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(km, ActionScrollDown); !reflect.DeepEqual(got, []string{"n"}) {
			t.Errorf("scroll_down keys = %v, want [n]", got)
		}
		if _, ok := km.Lookup(runeEvent('j')); ok {
			t.Error("j is still bound after scroll_down was overridden")
		}
	})

	t.Run("action names are case-insensitive", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{" Quit ": {"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(km, ActionQuit); !reflect.DeepEqual(got, []string{"x"}) {
			t.Errorf("quit keys = %v, want [x]", got)
		}
	})

	t.Run("many keys keep their order", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{"search": {"s", "/", "ctrl+s"}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(km, ActionSearch); !reflect.DeepEqual(got, []string{"s", "/", "ctrl+s"}) {
			t.Errorf("search keys = %v", got)
		}
	})

	t.Run("two actions swap keys by overriding both", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{
			"docker_view": {"a", "A"},
			"aws_view":    {"d", "D"},
		}})
		if err != nil {
			t.Fatalf("a swap of two actions should be accepted: %v", err)
		}
		if action, _ := km.Lookup(runeEvent('a')); action != ActionDockerView {
			t.Errorf("a = %q, want docker_view", action)
		}
		if action, _ := km.Lookup(runeEvent('d')); action != ActionAWSView {
			t.Errorf("d = %q, want aws_view", action)
		}
	})

	t.Run("a key may move to an action when the other one gives it up", func(t *testing.T) {
		if _, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{
			"quit": {"x"}, "help": {"q", "?"},
		}}); err != nil {
			t.Errorf("q moves from quit to help, no key is on two actions: %v", err)
		}
	})

	t.Run("a scroll key may stay on the matching action", func(t *testing.T) {
		km, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{
			"scroll_down": {"j", "down"}, "page_down": {"ctrl+f", "pgdn"}, "page_up": {"ctrl+b"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if action, _ := km.Lookup(keyEvent(tcell.KeyCtrlF)); action != ActionPageDown {
			t.Errorf("Ctrl+F = %q, want page_down", action)
		}
	})
}

func TestBuildKeymapErrors(t *testing.T) {
	actionList := strings.Join(ActionNames(), ", ")

	tests := []struct {
		name string
		cfg  config.KeybindingsConfig
		want []string
	}{
		{
			name: "emacs is not a preset",
			cfg:  config.KeybindingsConfig{Preset: "emacs"},
			want: []string{"keybindings.preset", `"emacs"`, "valid presets: default, vim"},
		},
		{
			name: "unknown action lists the valid ones",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"qiut": {"x"}}},
			want: []string{"keybindings.bindings.qiut", "unknown action", "valid actions: " + actionList},
		},
		{
			name: "empty key list",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {}}},
			want: []string{"keybindings.bindings.quit", "no keys", "at least one key"},
		},
		{
			name: "null key list",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": nil}},
			want: []string{"keybindings.bindings.quit", "no keys"},
		},
		{
			name: "invalid key names the key and the format",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"escape-hatch"}}},
			want: []string{"keybindings.bindings.quit", `"escape-hatch"`, "valid keys:", "ctrl+<letter>", "pgup"},
		},
		{
			name: "ctrl+c is reserved",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"q", "ctrl+c"}}},
			want: []string{"keybindings.bindings.quit", `"ctrl+c" is reserved`, "always quits"},
		},
		{
			name: "ctrl+c is reserved for every action",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"help": {"Ctrl+C"}}},
			want: []string{"keybindings.bindings.help", "reserved"},
		},
		{
			name: "ctrl+h is the backspace key",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"help": {"ctrl+h"}}},
			want: []string{"keybindings.bindings.help", "Backspace"},
		},
		{
			name: "the same key twice in one action",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x", "x"}}},
			want: []string{"keybindings.bindings.quit", `"x"`, "more than once"},
		},
		{
			name: "the same key spelled two ways in one action",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"clear_filter": {"esc", "Escape"}}},
			want: []string{"keybindings.bindings.clear_filter", "more than once"},
		},
		{
			name: "the same key on two actions",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}, "search": {"x"}}},
			want: []string{"keybindings:", `"x"`, `"quit" and "search"`, "override the other action too"},
		},
		{
			name: "an override clashes with a key of the preset",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"w"}}},
			want: []string{`"w"`, `"quit" and "toggle_alerts"`},
		},
		{
			name: "vim keys clash with an override",
			cfg:  config.KeybindingsConfig{Preset: "vim", Bindings: map[string][]string{"quit": {"j"}}},
			want: []string{`"j"`, `"quit" and "scroll_down"`},
		},
		{
			name: "a scroll key given to another action",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"up"}}},
			want: []string{"keybindings.bindings.quit", `"up"`, "handled by the panel with focus", `"scroll_up"`},
		},
		{
			name: "a scroll key given to the wrong scroll action",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"scroll_down": {"pgup"}}},
			want: []string{"keybindings.bindings.scroll_down", `"pgup"`, `"page_up"`},
		},
		{
			name: "ctrl+f is the panel's page down",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"scroll_down": {"ctrl+f"}}},
			want: []string{`"ctrl+f"`, "scroll down a page", `"page_down"`},
		},
		{
			name: "the same action written twice with different case",
			cfg:  config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"x"}, "QUIT": {"y"}}},
			want: []string{"keybindings.bindings.quit", `already set by "QUIT"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km, err := BuildKeymap(tt.cfg)
			if err == nil {
				t.Fatalf("BuildKeymap() = %+v, want an error", km)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if len(km.lookup) != 0 || len(km.keys) != 0 {
				t.Error("a failed BuildKeymap must return an empty keymap")
			}
		})
	}

	t.Run("errors are reported together, in a fixed order", func(t *testing.T) {
		cfg := config.KeybindingsConfig{
			Preset: "emacs",
			Bindings: map[string][]string{
				"zzz":    {"x"},
				"quit":   {"ctrl+c"},
				"help":   {},
				"search": {"nope"},
			},
		}
		_, err := BuildKeymap(cfg)
		if err == nil {
			t.Fatal("want an error")
		}
		lines := strings.Split(err.Error(), "\n")
		wantStart := []string{
			"keybindings.preset:",
			"keybindings.bindings.help:",
			"keybindings.bindings.quit:",
			"keybindings.bindings.search:",
			"keybindings.bindings.zzz:",
		}
		if len(lines) != len(wantStart) {
			t.Fatalf("got %d errors, want %d:\n%s", len(lines), len(wantStart), err)
		}
		for i, prefix := range wantStart {
			if !strings.HasPrefix(lines[i], prefix) {
				t.Errorf("error %d = %q, want it to start with %q", i, lines[i], prefix)
			}
		}
		// Map iteration order is random, so ask many times.
		for range 50 {
			if _, again := BuildKeymap(cfg); again.Error() != err.Error() {
				t.Fatalf("the error changed between calls:\n%s\n---\n%s", err, again)
			}
		}
	})

	t.Run("conflicts are reported by key, sorted, naming every action", func(t *testing.T) {
		cfg := config.KeybindingsConfig{Bindings: map[string][]string{
			"quit": {"x", "y"}, "search": {"y", "x"}, "help": {"x"},
		}}
		_, err := BuildKeymap(cfg)
		if err == nil {
			t.Fatal("want an error")
		}
		lines := strings.Split(err.Error(), "\n")
		if len(lines) != 2 {
			t.Fatalf("got %d errors, want one per conflicting key:\n%s", len(lines), err)
		}
		if !strings.Contains(lines[0], `"x"`) || !strings.Contains(lines[0], `"help", "quit" and "search"`) {
			t.Errorf("first conflict = %q", lines[0])
		}
		if !strings.Contains(lines[1], `"y"`) || !strings.Contains(lines[1], `"quit" and "search"`) {
			t.Errorf("second conflict = %q", lines[1])
		}
		for range 50 {
			if _, again := BuildKeymap(cfg); again.Error() != err.Error() {
				t.Fatal("the conflict messages changed between calls")
			}
		}
	})

	t.Run("keys that read still count for conflicts", func(t *testing.T) {
		_, err := BuildKeymap(config.KeybindingsConfig{Bindings: map[string][]string{"quit": {"w", "nope"}}})
		if err == nil {
			t.Fatal("want an error")
		}
		for _, want := range []string{`"nope"`, `"quit" and "toggle_alerts"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		}
	})
}

func TestKeymapKeysIsACopy(t *testing.T) {
	km := DefaultKeymap()
	keys := km.Keys(ActionQuit)
	keys[0] = runeCombo('z')
	if got := km.Keys(ActionQuit)[0]; got != runeCombo('q') {
		t.Errorf("mutating the slice from Keys changed the keymap: first key = %v", got)
	}
	if _, ok := (Keymap{}).Lookup(runeEvent('q')); ok {
		t.Error("the zero Keymap should bind nothing")
	}
}
