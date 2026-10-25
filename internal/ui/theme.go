package ui

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Theme assigns a color to every semantic role the dashboard draws with. A
// theme is a plain value: ResolveTheme builds one from a built-in theme plus
// the user's overrides, and the dashboard reads the roles from it.
type Theme struct {
	// ServicePalette colors the log text of each service, in container order;
	// it repeats when there are more services than colors. Use its colors only
	// to build tview tags (Tag, colorName): the dark theme holds marker values
	// in it (see colorLegacyCyan) that are not real tcell colors, so passing
	// one to SetTextColor or SetBackgroundColor draws nothing useful.
	ServicePalette []tcell.Color

	// General.
	Background tcell.Color // background of every panel
	Text       tcell.Color // regular text
	Muted      tcell.Color // secondary text such as timestamps and counters
	Accent     tcell.Color // table headers, labels and service names in the logs
	Border     tcell.Color // panel borders and table grid lines
	Title      tcell.Color // panel titles
	Info       tcell.Color // informational columns such as the image name
	Subheading tcell.Color // sub-headings such as "Recent Traces:"; tags only, may hold a legacy marker (see ServicePalette)
	Faint      tcell.Color // least important text, such as the retry notice

	// Status.
	StatusOK    tcell.Color
	StatusWarn  tcell.Color
	StatusError tcell.Color

	// Help bar and key hints.
	HelpBarBg tcell.Color
	HelpKey   tcell.Color
	HelpText  tcell.Color

	// Log level badges. The badge text is not drawn today: tview reads [INFO],
	// [WARN] and the like as color tags, as it did in the original dashboard,
	// and showing the badges would change the default look. So only LogFatal has
	// a visible effect, as the background of the rest of a FATAL line; the
	// other roles are reserved for when the badges are shown.
	LogFatal     tcell.Color // background set by the FATAL badge tag
	LogFatalText tcell.Color // text of the FATAL badge; no visible effect yet
	LogError     tcell.Color // no visible effect yet
	LogWarn      tcell.Color // no visible effect yet
	LogInfo      tcell.Color // no visible effect yet
	LogDebug     tcell.Color // DEBUG and TRACE; no visible effect yet

	// Search field.
	FieldBg   tcell.Color
	FieldHint tcell.Color // placeholder text
}

// colorRole links a role name, as written under theme.colors in the config
// file, to the Theme field it sets. ServicePalette is a list and has no role.
type colorRole struct {
	field func(*Theme) *tcell.Color
	name  string
}

// colorRoles lists every role in the order the documentation presents them.
var colorRoles = []colorRole{
	{name: "background", field: func(t *Theme) *tcell.Color { return &t.Background }},
	{name: "text", field: func(t *Theme) *tcell.Color { return &t.Text }},
	{name: "muted", field: func(t *Theme) *tcell.Color { return &t.Muted }},
	{name: "accent", field: func(t *Theme) *tcell.Color { return &t.Accent }},
	{name: "border", field: func(t *Theme) *tcell.Color { return &t.Border }},
	{name: "title", field: func(t *Theme) *tcell.Color { return &t.Title }},
	{name: "info", field: func(t *Theme) *tcell.Color { return &t.Info }},
	{name: "subheading", field: func(t *Theme) *tcell.Color { return &t.Subheading }},
	{name: "faint", field: func(t *Theme) *tcell.Color { return &t.Faint }},
	{name: "status_ok", field: func(t *Theme) *tcell.Color { return &t.StatusOK }},
	{name: "status_warn", field: func(t *Theme) *tcell.Color { return &t.StatusWarn }},
	{name: "status_error", field: func(t *Theme) *tcell.Color { return &t.StatusError }},
	{name: "help_bar_bg", field: func(t *Theme) *tcell.Color { return &t.HelpBarBg }},
	{name: "help_key", field: func(t *Theme) *tcell.Color { return &t.HelpKey }},
	{name: "help_text", field: func(t *Theme) *tcell.Color { return &t.HelpText }},
	{name: "log_fatal", field: func(t *Theme) *tcell.Color { return &t.LogFatal }},
	{name: "log_fatal_text", field: func(t *Theme) *tcell.Color { return &t.LogFatalText }},
	{name: "log_error", field: func(t *Theme) *tcell.Color { return &t.LogError }},
	{name: "log_warn", field: func(t *Theme) *tcell.Color { return &t.LogWarn }},
	{name: "log_info", field: func(t *Theme) *tcell.Color { return &t.LogInfo }},
	{name: "log_debug", field: func(t *Theme) *tcell.Color { return &t.LogDebug }},
	{name: "field_bg", field: func(t *Theme) *tcell.Color { return &t.FieldBg }},
	{name: "field_hint", field: func(t *Theme) *tcell.Color { return &t.FieldHint }},
}

// ColorRoleNames returns the role names accepted under theme.colors, in the
// order the documentation lists them.
func ColorRoleNames() []string {
	names := make([]string, len(colorRoles))
	for i, r := range colorRoles {
		names[i] = r.name
	}
	return names
}

// ThemeNames returns the built-in theme names in alphabetical order.
func ThemeNames() []string {
	themes := BuiltinThemes()
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuiltinThemes returns the four built-in themes by name. Every call returns
// fresh copies, so callers may modify the result.
func BuiltinThemes() map[string]Theme {
	return map[string]Theme{
		"dark":      darkTheme(),
		"light":     lightTheme(),
		"solarized": solarizedTheme(),
		"nord":      nordTheme(),
	}
}

// tview resolves a tag such as [cyan] through tcell.GetColor, which only knows
// the W3C names (aqua, fuchsia) and answers "no color" for cyan and magenta.
// So the original dashboard has always drawn the "cyan" and "magenta" service
// colors, and the "Recent Traces:" heading, in the terminal's default
// foreground. The dark theme must keep looking exactly like the original, so
// it spells those two tags the same way through these markers. They have no
// tcell color value of their own and are meant for tags only. Replace them with
// tcell.ColorAqua and tcell.ColorFuchsia in darkTheme to get real cyan and
// magenta.
const (
	colorLegacyCyan    = tcell.ColorSpecial | 0x7001
	colorLegacyMagenta = tcell.ColorSpecial | 0x7002
)

// darkTheme reproduces, exactly, the colors the dashboard used before themes
// existed.
func darkTheme() Theme {
	return Theme{
		Background: tcell.ColorBlack,
		Text:       tcell.ColorWhite,
		Muted:      tcell.ColorGray,
		Accent:     tcell.ColorYellow,
		Border:     tcell.ColorWhite,
		Title:      tcell.ColorWhite,
		Info:       tcell.ColorLightBlue,
		Subheading: colorLegacyCyan,
		Faint:      tcell.ColorDarkGray,

		StatusOK:    tcell.ColorGreen,
		StatusWarn:  tcell.ColorYellow,
		StatusError: tcell.ColorRed,

		HelpBarBg: tcell.ColorBlack,
		HelpKey:   tcell.ColorYellow,
		HelpText:  tcell.ColorWhite,

		LogFatal:     tcell.ColorRed,
		LogFatalText: tcell.ColorWhite,
		LogError:     tcell.ColorRed,
		LogWarn:      tcell.ColorYellow,
		LogInfo:      tcell.ColorGreen,
		LogDebug:     tcell.ColorDarkGray,

		FieldBg:   tcell.ColorBlue,
		FieldHint: tcell.ColorNavy,

		ServicePalette: []tcell.Color{
			tcell.ColorYellow, colorLegacyCyan, colorLegacyMagenta, tcell.ColorGreen, tcell.ColorBlue,
			tcell.ColorRed, tcell.ColorDarkCyan, tcell.ColorDarkMagenta, tcell.ColorOlive, tcell.ColorTeal,
		},
	}
}

func lightTheme() Theme {
	return Theme{
		Background: hexColor(0xffffff),
		Text:       hexColor(0x1f2328),
		Muted:      hexColor(0x6e7781),
		Accent:     hexColor(0x9a6700),
		Border:     hexColor(0x8c959f),
		Title:      hexColor(0x1f2328),
		Info:       hexColor(0x0550ae),
		Subheading: hexColor(0x0b6e99),
		Faint:      hexColor(0x8c959f),

		StatusOK:    hexColor(0x1a7f37),
		StatusWarn:  hexColor(0x9a6700),
		StatusError: hexColor(0xcf222e),

		HelpBarBg: hexColor(0xeaeef2),
		HelpKey:   hexColor(0x9a6700),
		HelpText:  hexColor(0x1f2328),

		LogFatal:     hexColor(0xcf222e),
		LogFatalText: hexColor(0xffffff),
		LogError:     hexColor(0xcf222e),
		LogWarn:      hexColor(0x9a6700),
		LogInfo:      hexColor(0x1a7f37),
		LogDebug:     hexColor(0x6e7781),

		FieldBg:   hexColor(0xeaeef2),
		FieldHint: hexColor(0x8c959f),

		ServicePalette: hexPalette(
			0x9a6700, 0x0969da, 0x8250df, 0x1a7f37, 0x0550ae,
			0xcf222e, 0x1b7c83, 0xbf3989, 0x953800, 0x57606a,
		),
	}
}

// solarizedTheme is Solarized Dark.
func solarizedTheme() Theme {
	return Theme{
		Background: hexColor(0x002b36),
		Text:       hexColor(0x839496),
		Muted:      hexColor(0x586e75),
		Accent:     hexColor(0xb58900),
		Border:     hexColor(0x586e75),
		Title:      hexColor(0x93a1a1),
		Info:       hexColor(0x268bd2),
		Subheading: hexColor(0x2aa198),
		Faint:      hexColor(0x586e75),

		StatusOK:    hexColor(0x859900),
		StatusWarn:  hexColor(0xb58900),
		StatusError: hexColor(0xdc322f),

		HelpBarBg: hexColor(0x073642),
		HelpKey:   hexColor(0xb58900),
		HelpText:  hexColor(0x93a1a1),

		LogFatal:     hexColor(0xdc322f),
		LogFatalText: hexColor(0xfdf6e3),
		LogError:     hexColor(0xdc322f),
		LogWarn:      hexColor(0xb58900),
		LogInfo:      hexColor(0x859900),
		LogDebug:     hexColor(0x586e75),

		FieldBg:   hexColor(0x073642),
		FieldHint: hexColor(0x586e75),

		ServicePalette: hexPalette(
			0xb58900, 0x2aa198, 0xd33682, 0x859900, 0x268bd2,
			0xdc322f, 0x6c71c4, 0xcb4b16, 0x93a1a1, 0x657b83,
		),
	}
}

func nordTheme() Theme {
	return Theme{
		Background: hexColor(0x2e3440),
		Text:       hexColor(0xd8dee9),
		Muted:      hexColor(0x7b88a1),
		Accent:     hexColor(0xebcb8b),
		Border:     hexColor(0x4c566a),
		Title:      hexColor(0x88c0d0),
		Info:       hexColor(0x81a1c1),
		Subheading: hexColor(0x88c0d0),
		Faint:      hexColor(0x616e88),

		StatusOK:    hexColor(0xa3be8c),
		StatusWarn:  hexColor(0xebcb8b),
		StatusError: hexColor(0xbf616a),

		HelpBarBg: hexColor(0x3b4252),
		HelpKey:   hexColor(0xebcb8b),
		HelpText:  hexColor(0xd8dee9),

		LogFatal:     hexColor(0xbf616a),
		LogFatalText: hexColor(0xeceff4),
		LogError:     hexColor(0xbf616a),
		LogWarn:      hexColor(0xebcb8b),
		LogInfo:      hexColor(0xa3be8c),
		LogDebug:     hexColor(0x616e88),

		FieldBg:   hexColor(0x3b4252),
		FieldHint: hexColor(0x7b88a1),

		ServicePalette: hexPalette(
			0xebcb8b, 0x88c0d0, 0xb48ead, 0xa3be8c, 0x81a1c1,
			0xbf616a, 0x8fbcbb, 0xd08770, 0x5e81ac, 0xe5e9f0,
		),
	}
}

func hexColor(rgb int32) tcell.Color { return tcell.NewHexColor(rgb) }

func hexPalette(rgb ...int32) []tcell.Color {
	palette := make([]tcell.Color, len(rgb))
	for i, v := range rgb {
		palette[i] = hexColor(v)
	}
	return palette
}

// hexPattern is the only accepted spelling of an RGB color.
var hexPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// colorAliases are names people expect to work that tcell spells differently.
var colorAliases = map[string]tcell.Color{
	"cyan":    tcell.ColorAqua,
	"magenta": tcell.ColorFuchsia,
}

// parseColor reads a color name known to tcell (such as "yellow" or "navy"),
// cyan and magenta, or a #rrggbb value. It is case-insensitive.
func parseColor(value string) (tcell.Color, bool) {
	s := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(s, "#") {
		// tcell would also take "#-12345" as a (garbled) color, so the only
		// accepted spelling is checked here.
		if hexPattern.MatchString(s) {
			return tcell.GetColor(s), true
		}
		return tcell.ColorDefault, false
	}
	if c, ok := colorAliases[s]; ok {
		return c, true
	}
	// GetColor answers ColorDefault for a name it does not know.
	if c := tcell.GetColor(s); c != tcell.ColorDefault {
		return c, true
	}
	return tcell.ColorDefault, false
}

// ResolveTheme builds the theme named in cfg (dark when the name is empty) and
// applies the per-role color overrides on top. Unknown theme names, unknown
// roles and colors that cannot be read are all reported together.
func ResolveTheme(cfg config.ThemeConfig) (Theme, error) {
	var errs []error

	name := strings.ToLower(strings.TrimSpace(cfg.Name))
	if name == "" {
		name = config.DefaultThemeName
	}
	theme, ok := BuiltinThemes()[name]
	if !ok {
		errs = append(errs, fmt.Errorf("theme: no built-in theme named %q (valid themes: %s)", cfg.Name, strings.Join(ThemeNames(), ", ")))
		theme = darkTheme()
	}

	roles := make(map[string]colorRole, len(colorRoles))
	for _, r := range colorRoles {
		roles[r.name] = r
	}

	keys := make([]string, 0, len(cfg.Colors))
	for key := range cfg.Colors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		role, known := roles[key]
		if !known {
			errs = append(errs, fmt.Errorf("theme.colors.%s: unknown color role (valid roles: %s)", key, strings.Join(ColorRoleNames(), ", ")))
			continue
		}
		color, valid := parseColor(cfg.Colors[key])
		switch {
		case !valid && strings.TrimSpace(cfg.Colors[key]) == "":
			// An unquoted #rrggbb in YAML is a comment, which leaves the value empty.
			errs = append(errs, fmt.Errorf("theme.colors.%s: empty color (quote #rrggbb values in the YAML file, because an unquoted # starts a comment)", key))
			continue
		case !valid:
			errs = append(errs, fmt.Errorf("theme.colors.%s: invalid color %q (use a color name such as yellow or navy, or #rrggbb)", key, cfg.Colors[key]))
			continue
		}
		*role.field(&theme) = color
	}

	if err := errors.Join(errs...); err != nil {
		return Theme{}, err
	}
	return theme, nil
}

// tagNames spells the colors the dashboard has always used in tview tags by
// name instead of by value, so the dark theme produces the very same tags as
// before and terminals with a custom palette keep showing their own colors.
var tagNames = map[tcell.Color]string{
	tcell.ColorYellow:      "yellow",
	tcell.ColorWhite:       "white",
	tcell.ColorRed:         "red",
	tcell.ColorGreen:       "green",
	tcell.ColorGray:        "gray",
	tcell.ColorDarkGray:    "darkgray",
	tcell.ColorBlue:        "blue",
	tcell.ColorDarkCyan:    "darkcyan",
	tcell.ColorDarkMagenta: "darkmagenta",
	tcell.ColorOlive:       "olive",
	tcell.ColorTeal:        "teal",
	colorLegacyCyan:        "cyan",
	colorLegacyMagenta:     "magenta",
}

// colorName is how a color is written inside a tview tag: a name from tagNames,
// #rrggbb for any other color, or "-" (reset) for the default color.
func colorName(c tcell.Color) string {
	if name, ok := tagNames[c]; ok {
		return name
	}
	if hex := c.Hex(); hex >= 0 {
		return fmt.Sprintf("#%06x", hex)
	}
	return "-"
}

// Tag returns the tview tag that switches the text foreground to c, such as
// "[yellow]" or "[#ff8800]". The default color gives "[-]", which resets to
// the text view's base color.
func (Theme) Tag(c tcell.Color) string {
	return "[" + colorName(c) + "]"
}

// pristineStyles holds tview's own default styles, captured before anything in
// this package can change them. Apply always starts from these, so applying a
// theme never depends on which theme was applied before.
var pristineStyles = tview.Styles

// Apply sets tview's global styles from the theme. It must run before the
// primitives are created, because tview reads the styles when a primitive is
// built. For the dark theme the styles end up equal to tview's defaults.
func (t Theme) Apply() {
	s := pristineStyles
	s.PrimitiveBackgroundColor = t.Background
	s.ContrastBackgroundColor = t.FieldBg
	s.BorderColor = t.Border
	s.GraphicsColor = t.Border
	s.TitleColor = t.Title
	s.PrimaryTextColor = t.Text
	s.SecondaryTextColor = t.Accent
	s.ContrastSecondaryTextColor = t.FieldHint
	tview.Styles = s
}
