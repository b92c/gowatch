package ui

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
)

// Action is something the user can do from the keyboard. The names are the ones
// written under keybindings.bindings in the config file.
type Action string

// The actions of the dashboard.
const (
	ActionHelp                Action = "help"
	ActionQuit                Action = "quit"
	ActionDockerView          Action = "docker_view"
	ActionAWSView             Action = "aws_view"
	ActionSearch              Action = "search"
	ActionCycleLogLevel       Action = "cycle_log_level"
	ActionClearFilter         Action = "clear_filter"
	ActionToggleAlerts        Action = "toggle_alerts"
	ActionClearResolvedAlerts Action = "clear_resolved_alerts"
	ActionScrollUp            Action = "scroll_up"
	ActionScrollDown          Action = "scroll_down"
	ActionPageUp              Action = "page_up"
	ActionPageDown            Action = "page_down"
	ActionScrollTop           Action = "scroll_top"
	ActionScrollBottom        Action = "scroll_bottom"
)

// The groups the help page sorts the actions into, in the order it shows them.
const (
	groupGeneral    = "General"
	groupViews      = "Views"
	groupFilters    = "Filters"
	groupAlerts     = "Alerts"
	groupNavigation = "Navigation"
)

// actionInfo is what the help page knows about an action.
type actionInfo struct {
	action      Action
	group       string
	description string
}

// actionTable lists every action, grouped, in the order the help page shows
// them.
var actionTable = []actionInfo{
	{action: ActionHelp, group: groupGeneral, description: "Show or hide this help"},
	{action: ActionQuit, group: groupGeneral, description: "Quit GoWatch"},
	{action: ActionDockerView, group: groupViews, description: "Show the Docker services"},
	{action: ActionAWSView, group: groupViews, description: "Show the AWS resources"},
	{action: ActionSearch, group: groupFilters, description: "Search by service name, ID or image"},
	{action: ActionCycleLogLevel, group: groupFilters, description: "Cycle the minimum log level"},
	{action: ActionClearFilter, group: groupFilters, description: "Clear all filters (closes the alerts list first)"},
	{action: ActionToggleAlerts, group: groupAlerts, description: "Show or hide the alerts list"},
	{action: ActionClearResolvedAlerts, group: groupAlerts, description: "Clear the resolved alerts"},
	{action: ActionScrollUp, group: groupNavigation, description: "Scroll up one line"},
	{action: ActionScrollDown, group: groupNavigation, description: "Scroll down one line"},
	{action: ActionPageUp, group: groupNavigation, description: "Scroll up one page"},
	{action: ActionPageDown, group: groupNavigation, description: "Scroll down one page"},
	{action: ActionScrollTop, group: groupNavigation, description: "Jump to the top"},
	{action: ActionScrollBottom, group: groupNavigation, description: "Jump to the bottom"},
}

// groupOrder is the order of the groups in the help page.
var groupOrder = []string{groupGeneral, groupViews, groupFilters, groupAlerts, groupNavigation}

// Actions returns every action, grouped, in the order the help page lists them.
func Actions() []Action {
	actions := make([]Action, len(actionTable))
	for i, info := range actionTable {
		actions[i] = info.action
	}
	return actions
}

// ActionNames returns the action names accepted under keybindings.bindings, in
// the order the help page lists them.
func ActionNames() []string {
	names := make([]string, len(actionTable))
	for i, info := range actionTable {
		names[i] = string(info.action)
	}
	return names
}

func (a Action) info() (actionInfo, bool) {
	for _, info := range actionTable {
		if info.action == a {
			return info, true
		}
	}
	return actionInfo{}, false
}

// Group is the help page section the action belongs to; empty for an unknown
// action.
func (a Action) Group() string {
	info, _ := a.info()
	return info.group
}

// Description is a short sentence about what the action does; empty for an
// unknown action.
func (a Action) Description() string {
	info, _ := a.info()
	return info.description
}

// KeyCombo is a key a binding can name: a rune (Rune is set and Key is
// tcell.KeyRune) or a special key such as Esc or Ctrl+D (Key is set, Rune is
// zero). Modifiers are not part of it: the dashboard has always ignored them,
// so Alt+q still quits.
type KeyCombo struct {
	Rune rune
	Key  tcell.Key
}

// comboOf is the KeyCombo an incoming key event stands for.
func comboOf(ev *tcell.EventKey) KeyCombo {
	if ev.Key() == tcell.KeyRune {
		return KeyCombo{Key: tcell.KeyRune, Rune: ev.Rune()}
	}
	return KeyCombo{Key: ev.Key()}
}

func runeCombo(r rune) KeyCombo { return KeyCombo{Key: tcell.KeyRune, Rune: r} }

// namedKey ties a key name accepted in the config file to its tcell key and to
// how the dashboard writes it on screen.
type namedKey struct {
	name  string
	label string
	key   tcell.Key
}

var namedKeys = []namedKey{
	{name: "esc", label: "Esc", key: tcell.KeyEscape},
	{name: "enter", label: "Enter", key: tcell.KeyEnter},
	{name: "tab", label: "Tab", key: tcell.KeyTab},
	{name: "up", label: "↑", key: tcell.KeyUp},
	{name: "down", label: "↓", key: tcell.KeyDown},
	{name: "pgup", label: "PgUp", key: tcell.KeyPgUp},
	{name: "pgdn", label: "PgDn", key: tcell.KeyPgDn},
	{name: "home", label: "Home", key: tcell.KeyHome},
	{name: "end", label: "End", key: tcell.KeyEnd},
}

// keyAliases are other spellings of the names above.
var keyAliases = map[string]string{"escape": "esc"}

// keyFormat is the accepted key syntax, as shown in error messages.
const keyFormat = "a single character such as q, / or G (upper and lower case are different keys), " +
	"ctrl+<letter>, or one of esc, enter, tab, up, down, pgup, pgdn, home, end, space"

// ctrlLetter returns the letter of a Ctrl+<letter> key.
func ctrlLetter(k tcell.Key) (rune, bool) {
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		return 'a' + rune(k-tcell.KeyCtrlA), true
	}
	return 0, false
}

// ParseKey reads a key as written in the config file: a single character
// (q, /, ?, G, ...; upper and lower case are different keys), ctrl+<letter>,
// space, or one of esc (or escape), enter, tab, up, down, pgup, pgdn, home and
// end. The names are case-insensitive.
//
// ctrl+h, ctrl+i and ctrl+m are refused: a terminal sends the very same bytes
// as for Backspace, Tab and Enter, so they could never be told apart.
func ParseKey(s string) (KeyCombo, error) {
	text := strings.TrimSpace(s)
	if utf8.RuneCountInString(s) == 1 {
		text = s // a lone space is the space key
	}
	if text == "" {
		return KeyCombo{}, fmt.Errorf("empty key (valid keys: %s)", keyFormat)
	}

	if utf8.RuneCountInString(text) == 1 {
		r, _ := utf8.DecodeRuneInString(text)
		if unicode.IsControl(r) || r == utf8.RuneError {
			return KeyCombo{}, fmt.Errorf("key %q is a control character; write it as tab, enter, esc or ctrl+<letter>", s)
		}
		return runeCombo(r), nil
	}

	name := strings.ToLower(text)
	if canonical, ok := keyAliases[name]; ok {
		name = canonical
	}
	if name == "space" {
		return runeCombo(' '), nil
	}
	for _, nk := range namedKeys {
		if nk.name == name {
			return KeyCombo{Key: nk.key}, nil
		}
	}
	if letter, ok := strings.CutPrefix(name, "ctrl+"); ok && len(letter) == 1 && letter[0] >= 'a' && letter[0] <= 'z' {
		switch letter {
		case "h":
			return KeyCombo{}, errors.New(`"ctrl+h" cannot be bound: a terminal sends Backspace for it`)
		case "i":
			return KeyCombo{}, errors.New(`"ctrl+i" cannot be bound: a terminal sends Tab for it (use "tab")`)
		case "m":
			return KeyCombo{}, errors.New(`"ctrl+m" cannot be bound: a terminal sends Enter for it (use "enter")`)
		}
		return KeyCombo{Key: tcell.KeyCtrlA + tcell.Key(letter[0]-'a')}, nil
	}
	return KeyCombo{}, fmt.Errorf("unknown key %q (valid keys: %s)", s, keyFormat)
}

// String is the key as ParseKey reads it.
func (k KeyCombo) String() string {
	if k.Key == tcell.KeyRune {
		if k.Rune == ' ' {
			return "space"
		}
		return string(k.Rune)
	}
	for _, nk := range namedKeys {
		if nk.key == k.Key {
			return nk.name
		}
	}
	if letter, ok := ctrlLetter(k.Key); ok {
		return "ctrl+" + string(letter)
	}
	return fmt.Sprintf("key(%d)", k.Key)
}

// Label is the key as the dashboard shows it to the user: Esc, ↑, PgUp, Ctrl+D,
// or the character itself.
func (k KeyCombo) Label() string {
	if k.Key == tcell.KeyRune {
		if k.Rune == ' ' {
			return "Space"
		}
		return string(k.Rune)
	}
	for _, nk := range namedKeys {
		if nk.key == k.Key {
			return nk.label
		}
	}
	if letter, ok := ctrlLetter(k.Key); ok {
		return "Ctrl+" + strings.ToUpper(string(letter))
	}
	return k.String()
}

// reservedKey is Ctrl+C. tview closes the application on it when nothing else
// consumed the event, so no action may take it.
var reservedKey = KeyCombo{Key: tcell.KeyCtrlC}

// scrollKeys are the keys the focused panel (logs, a table) already scrolls
// with. The dashboard never takes them over: it hands them to the panel, so
// they work the same whatever the key bindings are. Each can only be bound to
// the action that matches what the panel does with it, which is also how the
// help page and the help bar learn about them.
var scrollKeys = map[tcell.Key]Action{
	tcell.KeyUp:    ActionScrollUp,
	tcell.KeyDown:  ActionScrollDown,
	tcell.KeyPgUp:  ActionPageUp,
	tcell.KeyPgDn:  ActionPageDown,
	tcell.KeyHome:  ActionScrollTop,
	tcell.KeyEnd:   ActionScrollBottom,
	tcell.KeyCtrlB: ActionPageUp,
	tcell.KeyCtrlF: ActionPageDown,
}

// scrollKeyOf is the key a panel understands for a scroll action, used to drive
// the tables of the help page and the alerts list with the action's own keys.
var scrollKeyOf = map[Action]tcell.Key{
	ActionScrollUp:     tcell.KeyUp,
	ActionScrollDown:   tcell.KeyDown,
	ActionPageUp:       tcell.KeyPgUp,
	ActionPageDown:     tcell.KeyPgDn,
	ActionScrollTop:    tcell.KeyHome,
	ActionScrollBottom: tcell.KeyEnd,
}

// isScrollAction reports whether a is one of the navigation actions.
func isScrollAction(a Action) bool {
	_, ok := scrollKeyOf[a]
	return ok
}

// Keymap says which key runs which action. Build one with BuildKeymap or take
// DefaultKeymap; the zero value binds nothing.
type Keymap struct {
	keys   map[Action][]KeyCombo
	lookup map[KeyCombo]Action
}

// newKeymap indexes bindings, which must not give one key to two actions.
func newKeymap(bindings map[Action][]KeyCombo) Keymap {
	km := Keymap{
		keys:   make(map[Action][]KeyCombo, len(bindings)),
		lookup: make(map[KeyCombo]Action),
	}
	for action, keys := range bindings {
		km.keys[action] = slices.Clone(keys)
		for _, k := range keys {
			km.lookup[k] = action
		}
	}
	return km
}

// Lookup returns the action bound to the key of the event. Modifiers are not
// looked at, as in every version of the dashboard so far.
func (km Keymap) Lookup(ev *tcell.EventKey) (Action, bool) {
	action, ok := km.lookup[comboOf(ev)]
	return action, ok
}

// Keys returns the keys bound to an action, in the order they were listed: the
// first one is the one the help bar shows.
func (km Keymap) Keys(a Action) []KeyCombo {
	return slices.Clone(km.keys[a])
}

// firstKey returns the first key of an action.
func (km Keymap) firstKey(a Action) (KeyCombo, bool) {
	keys := km.keys[a]
	if len(keys) == 0 {
		return KeyCombo{}, false
	}
	return keys[0], true
}

const (
	presetDefault = "default"
	presetVim     = "vim"
)

// defaultPreset is the keymap GoWatch has always had, with ? added for the help
// page. Upper and lower case letters are different keys, so both are listed.
// The scroll actions list the keys the panels already handle (see scrollKeys).
var defaultPreset = map[Action][]string{
	ActionHelp:                {"?"},
	ActionQuit:                {"q"},
	ActionDockerView:          {"d", "D"},
	ActionAWSView:             {"a", "A"},
	ActionSearch:              {"/", "f"},
	ActionCycleLogLevel:       {"l", "L"},
	ActionClearFilter:         {"esc"},
	ActionToggleAlerts:        {"w", "W"},
	ActionClearResolvedAlerts: {"c", "C"},
	ActionScrollUp:            {"up"},
	ActionScrollDown:          {"down"},
	ActionPageUp:              {"pgup"},
	ActionPageDown:            {"pgdn"},
	ActionScrollTop:           {"home"},
	ActionScrollBottom:        {"end"},
}

// vimExtras are the keys the vim preset adds to the default preset.
var vimExtras = map[Action][]string{
	ActionScrollDown:   {"j"},
	ActionScrollUp:     {"k"},
	ActionScrollTop:    {"g"},
	ActionScrollBottom: {"G"},
	ActionPageDown:     {"ctrl+d"},
	ActionPageUp:       {"ctrl+u"},
}

// presets are the built-in keymaps by name, parsed once.
var presets = map[string]map[Action][]KeyCombo{
	presetDefault: mustParseBindings(defaultPreset, nil),
	presetVim:     mustParseBindings(defaultPreset, vimExtras),
}

// mustParseBindings parses the key spellings of a preset. A preset that does not
// parse is a programming error, which the tests would catch.
func mustParseBindings(base, extra map[Action][]string) map[Action][]KeyCombo {
	bindings := make(map[Action][]KeyCombo, len(base))
	for _, source := range []map[Action][]string{base, extra} {
		for action, spellings := range source {
			for _, s := range spellings {
				k, err := ParseKey(s)
				if err != nil {
					panic(fmt.Sprintf("ui: built-in key %q of %s: %v", s, action, err))
				}
				bindings[action] = append(bindings[action], k)
			}
		}
	}
	return bindings
}

// PresetNames returns the names of the built-in presets.
func PresetNames() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultKeymap is the keymap of the default preset: the keys GoWatch has always
// had, plus ? for the help page.
func DefaultKeymap() Keymap { return newKeymap(presets[presetDefault]) }

// BuildKeymap builds the keymap of the preset named in cfg (default when empty)
// with the overrides of cfg.Bindings on top. An override replaces all the keys
// of its action. The names are case-insensitive.
//
// Every problem is reported at once: an unknown preset or action, an action
// with no keys, a key that cannot be read, Ctrl+C (reserved: it always quits),
// a key listed twice for one action, a scroll key (arrows, PgUp/PgDn,
// Home/End, Ctrl+F/Ctrl+B) given to an action other than the one the focused
// panel already runs on it, and a key left on two actions in the final keymap.
// The last check looks at preset and overrides together, so two actions swap
// keys by overriding both.
func BuildKeymap(cfg config.KeybindingsConfig) (Keymap, error) {
	var errs []error

	presetName := strings.ToLower(strings.TrimSpace(cfg.Preset))
	if presetName == "" {
		presetName = presetDefault
	}
	preset, ok := presets[presetName]
	if !ok {
		errs = append(errs, fmt.Errorf("keybindings.preset: unknown preset %q (valid presets: %s)", cfg.Preset, strings.Join(PresetNames(), ", ")))
		preset = presets[presetDefault]
	}
	bindings := make(map[Action][]KeyCombo, len(preset))
	for action, keys := range preset {
		bindings[action] = slices.Clone(keys)
	}

	rawNames := make([]string, 0, len(cfg.Bindings))
	for name := range cfg.Bindings {
		rawNames = append(rawNames, name)
	}
	sort.Strings(rawNames)

	given := make(map[Action]string, len(rawNames))
	for _, rawName := range rawNames {
		action := Action(strings.ToLower(strings.TrimSpace(rawName)))
		if _, known := action.info(); !known {
			errs = append(errs, fmt.Errorf("keybindings.bindings.%s: unknown action (valid actions: %s)", rawName, strings.Join(ActionNames(), ", ")))
			continue
		}
		if first, dup := given[action]; dup {
			errs = append(errs, fmt.Errorf("keybindings.bindings.%s: the action %q is already set by %q", rawName, action, first))
			continue
		}
		given[action] = rawName

		keys, keyErrs := parseOverride(rawName, action, cfg.Bindings[rawName])
		errs = append(errs, keyErrs...)
		// With problems in the list, the keys that did read still stand in for the
		// preset's, so a clash they cause is reported in the same run.
		if len(keys) > 0 {
			bindings[action] = keys
		}
	}

	errs = append(errs, keyConflicts(bindings)...)

	if err := errors.Join(errs...); err != nil {
		return Keymap{}, err
	}
	return newKeymap(bindings), nil
}

// parseOverride reads the key list of one action. It returns the keys that are
// valid along with an error for each one that is not.
func parseOverride(rawName string, action Action, spellings []string) ([]KeyCombo, []error) {
	where := "keybindings.bindings." + rawName
	if len(spellings) == 0 {
		return nil, []error{fmt.Errorf("%s: no keys (use at least one key, for example [\"q\"])", where)}
	}

	var (
		keys []KeyCombo
		errs []error
	)
	for _, spelling := range spellings {
		k, err := ParseKey(spelling)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		if k == reservedKey {
			errs = append(errs, fmt.Errorf("%s: %q is reserved: Ctrl+C always quits GoWatch and cannot be rebound", where, spelling))
			continue
		}
		if native, isScroll := scrollKeys[k.Key]; isScroll && native != action {
			errs = append(errs, fmt.Errorf("%s: %q is handled by the panel with focus, which always uses it to %s, so it can only be bound to %q", where, spelling, scrollVerb(native), native))
			continue
		}
		if slices.Contains(keys, k) {
			errs = append(errs, fmt.Errorf("%s: the key %q is listed more than once", where, spelling))
			continue
		}
		keys = append(keys, k)
	}
	return keys, errs
}

// scrollVerb says what a panel does with the scroll key of an action.
func scrollVerb(a Action) string {
	switch a {
	case ActionScrollUp:
		return "scroll up a line"
	case ActionScrollDown:
		return "scroll down a line"
	case ActionPageUp:
		return "scroll up a page"
	case ActionPageDown:
		return "scroll down a page"
	case ActionScrollTop:
		return "jump to the top"
	default:
		return "jump to the bottom"
	}
}

// keyConflicts reports each key that more than one action ends up with.
func keyConflicts(bindings map[Action][]KeyCombo) []error {
	owners := make(map[KeyCombo][]string)
	for action, keys := range bindings {
		for _, k := range keys {
			owners[k] = append(owners[k], string(action))
		}
	}

	conflicted := make([]KeyCombo, 0)
	for k, actions := range owners {
		if len(actions) > 1 {
			sort.Strings(actions)
			conflicted = append(conflicted, k)
		}
	}
	sort.Slice(conflicted, func(i, j int) bool { return conflicted[i].String() < conflicted[j].String() })

	errs := make([]error, 0, len(conflicted))
	for _, k := range conflicted {
		quoted := make([]string, len(owners[k]))
		for i, name := range owners[k] {
			quoted[i] = fmt.Sprintf("%q", name)
		}
		errs = append(errs, fmt.Errorf("keybindings: the key %q is bound to %s; a key can run only one action (to move a key to another action, or to swap keys between actions, override the other action too in keybindings.bindings)", k.String(), joinAnd(quoted)))
	}
	return errs
}

// joinAnd writes a list as "a and b" or "a, b and c".
func joinAnd(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
