package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// roleColors lists the color of every role of t by role name.
func roleColors(t Theme) map[string]tcell.Color {
	colors := make(map[string]tcell.Color, len(colorRoles))
	for _, r := range colorRoles {
		colors[r.name] = *r.field(&t)
	}
	return colors
}

func TestBuiltinThemesAreComplete(t *testing.T) {
	themes := BuiltinThemes()

	if got := ThemeNames(); !reflect.DeepEqual(got, []string{"dark", "light", "nord", "solarized"}) {
		t.Fatalf("ThemeNames() = %v, want the four built-in themes", got)
	}
	for name, theme := range themes {
		t.Run(name, func(t *testing.T) {
			for role, color := range roleColors(theme) {
				if color == tcell.ColorDefault {
					t.Errorf("role %s is not set", role)
				}
			}
			if len(theme.ServicePalette) != 10 {
				t.Errorf("service palette has %d colors, want 10", len(theme.ServicePalette))
			}
			seen := map[tcell.Color]bool{}
			for i, color := range theme.ServicePalette {
				if color == tcell.ColorDefault {
					t.Errorf("service palette color %d is not set", i)
				}
				if seen[color] {
					t.Errorf("service palette color %d (%v) repeats an earlier one", i, color)
				}
				seen[color] = true
			}
			if theme.Text == theme.Background {
				t.Errorf("text and background are both %v", theme.Text)
			}
			if theme.HelpText == theme.HelpBarBg {
				t.Errorf("help text and help bar background are both %v", theme.HelpText)
			}
			if theme.StatusOK == theme.StatusError {
				t.Errorf("OK and error status are both %v", theme.StatusOK)
			}
		})
	}
}

// Adding a Theme field without registering a role would make it impossible to
// configure, so every field except the palette must have exactly one role.
func TestEveryThemeFieldHasARole(t *testing.T) {
	var theme Theme
	typ := reflect.TypeOf(theme)
	if got, want := len(colorRoles), typ.NumField()-1; got != want {
		t.Errorf("%d color roles for %d single-color Theme fields", got, want)
	}

	seen := map[*tcell.Color]string{}
	names := map[string]bool{}
	for _, r := range colorRoles {
		ptr := r.field(&theme)
		if other, dup := seen[ptr]; dup {
			t.Errorf("roles %s and %s set the same field", other, r.name)
		}
		seen[ptr] = r.name
		if names[r.name] {
			t.Errorf("role name %s is listed twice", r.name)
		}
		names[r.name] = true
		if r.name != strings.ToLower(r.name) || strings.ContainsAny(r.name, " -") {
			t.Errorf("role name %q should be lower snake_case", r.name)
		}
	}
	if got := ColorRoleNames(); len(got) != len(colorRoles) || got[0] != colorRoles[0].name {
		t.Errorf("ColorRoleNames() = %v", got)
	}
}

func TestBuiltinThemesReturnIndependentCopies(t *testing.T) {
	first := BuiltinThemes()
	first["dark"].ServicePalette[0] = tcell.ColorPink
	delete(first, "light")

	second := BuiltinThemes()
	if second["dark"].ServicePalette[0] != tcell.ColorYellow {
		t.Error("changing a returned palette changed the next call's theme")
	}
	if _, ok := second["light"]; !ok {
		t.Error("deleting from a returned map changed the next call")
	}
}

func TestResolveTheme(t *testing.T) {
	themes := BuiltinThemes()

	t.Run("every built-in theme resolves to itself", func(t *testing.T) {
		for name, want := range themes {
			got, err := ResolveTheme(config.ThemeConfig{Name: name})
			if err != nil {
				t.Fatalf("ResolveTheme(%q) error = %v", name, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ResolveTheme(%q) = %+v, want %+v", name, got, want)
			}
		}
	})

	t.Run("names are case-insensitive and trimmed", func(t *testing.T) {
		got, err := ResolveTheme(config.ThemeConfig{Name: "  Solarized "})
		if err != nil || !reflect.DeepEqual(got, themes["solarized"]) {
			t.Errorf("ResolveTheme() = %+v, %v, want the solarized theme", got, err)
		}
	})

	t.Run("an empty config is the built-in dark theme", func(t *testing.T) {
		for _, cfg := range []config.ThemeConfig{{}, {Name: " "}, {Colors: map[string]string{}}} {
			got, err := ResolveTheme(cfg)
			if err != nil || !reflect.DeepEqual(got, themes[config.DefaultThemeName]) {
				t.Errorf("ResolveTheme(%+v) = %+v, %v, want the %s theme", cfg, got, err, config.DefaultThemeName)
			}
		}
	})
}

func TestResolveThemeOverrides(t *testing.T) {
	tests := []struct {
		colors map[string]string
		check  func(Theme) (got, want tcell.Color)
		name   string
		theme  string
	}{
		{name: "hex color", theme: "dark", colors: map[string]string{"accent": "#ff8800"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Accent, tcell.NewHexColor(0xff8800)
		}},
		{name: "uppercase hex", theme: "dark", colors: map[string]string{"accent": "#FF8800"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Accent, tcell.NewHexColor(0xff8800)
		}},
		{name: "color name", theme: "dark", colors: map[string]string{"status_error": "navy"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.StatusError, tcell.ColorNavy
		}},
		{name: "names are case-insensitive", theme: "dark", colors: map[string]string{"muted": " Yellow "}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Muted, tcell.ColorYellow
		}},
		{name: "cyan is an alias of aqua", theme: "dark", colors: map[string]string{"info": "cyan"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Info, tcell.ColorAqua
		}},
		{name: "magenta is an alias of fuchsia", theme: "dark", colors: map[string]string{"info": "magenta"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Info, tcell.ColorFuchsia
		}},
		{name: "on top of another theme", theme: "light", colors: map[string]string{"background": "#101010"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Background, tcell.NewHexColor(0x101010)
		}},
		{name: "a role that is not overridden keeps the theme color", theme: "nord", colors: map[string]string{"accent": "red"}, check: func(th Theme) (tcell.Color, tcell.Color) {
			return th.Text, nordTheme().Text
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTheme(config.ThemeConfig{Name: tt.theme, Colors: tt.colors})
			if err != nil {
				t.Fatalf("ResolveTheme() error = %v", err)
			}
			if have, want := tt.check(got); have != want {
				t.Errorf("color = %v, want %v", have, want)
			}
		})
	}

	t.Run("every role can be overridden", func(t *testing.T) {
		colors := map[string]string{}
		for _, name := range ColorRoleNames() {
			colors[name] = "#010203"
		}
		got, err := ResolveTheme(config.ThemeConfig{Colors: colors})
		if err != nil {
			t.Fatalf("ResolveTheme() error = %v", err)
		}
		for role, color := range roleColors(got) {
			if color != tcell.NewHexColor(0x010203) {
				t.Errorf("role %s = %v, want the override", role, color)
			}
		}
	})

	t.Run("the built-in theme is not modified", func(t *testing.T) {
		if _, err := ResolveTheme(config.ThemeConfig{Colors: map[string]string{"accent": "#ff8800"}}); err != nil {
			t.Fatal(err)
		}
		if BuiltinThemes()["dark"].Accent != tcell.ColorYellow {
			t.Error("an override leaked into the built-in dark theme")
		}
	})
}

func TestResolveThemeErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ThemeConfig
		want []string
	}{
		{"unknown theme", config.ThemeConfig{Name: "neon"}, []string{`no built-in theme named "neon"`, "dark, light, nord, solarized"}},
		{"unknown role", config.ThemeConfig{Colors: map[string]string{"acent": "red"}}, []string{"theme.colors.acent", "unknown color role", "accent", "status_error"}},
		{"role names are exact", config.ThemeConfig{Colors: map[string]string{"Accent": "red"}}, []string{"theme.colors.Accent", "unknown color role"}},
		{"not a color name", config.ThemeConfig{Colors: map[string]string{"accent": "not-a-color"}}, []string{"theme.colors.accent", `"not-a-color"`, "#rrggbb"}},
		{"empty color", config.ThemeConfig{Colors: map[string]string{"accent": ""}}, []string{"theme.colors.accent", "empty color", "quote #rrggbb", "unquoted # starts a comment"}},
		{"blank color", config.ThemeConfig{Colors: map[string]string{"accent": "  "}}, []string{"theme.colors.accent", "quote #rrggbb"}},
		{"short hex", config.ThemeConfig{Colors: map[string]string{"accent": "#fff"}}, []string{"theme.colors.accent", `"#fff"`}},
		{"long hex", config.ThemeConfig{Colors: map[string]string{"accent": "#ff88001"}}, []string{"theme.colors.accent"}},
		{"hex digits only", config.ThemeConfig{Colors: map[string]string{"accent": "#gg8800"}}, []string{"theme.colors.accent"}},
		{"sign inside the hex", config.ThemeConfig{Colors: map[string]string{"accent": "#-12345"}}, []string{"theme.colors.accent"}},
		{"hex without the hash", config.ThemeConfig{Colors: map[string]string{"accent": "ff8800"}}, []string{"theme.colors.accent"}},
		{"default is not a color", config.ThemeConfig{Colors: map[string]string{"accent": "default"}}, []string{"theme.colors.accent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTheme(tt.cfg)
			if err == nil {
				t.Fatalf("ResolveTheme() = %+v, want an error", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}

	t.Run("every problem is reported at once, in a stable order", func(t *testing.T) {
		cfg := config.ThemeConfig{
			Name:   "neon",
			Colors: map[string]string{"zzz": "red", "accent": "bad", "muted": "worse", "aaa": "red"},
		}
		_, err := ResolveTheme(cfg)
		if err == nil {
			t.Fatal("ResolveTheme() returned no error")
		}
		msg := err.Error()
		order := []string{`no built-in theme named "neon"`, "theme.colors.aaa", "theme.colors.accent", "theme.colors.muted", "theme.colors.zzz"}
		pos := -1
		for _, part := range order {
			i := strings.Index(msg, part)
			if i < 0 {
				t.Fatalf("error does not mention %q:\n%s", part, msg)
			}
			if i < pos {
				t.Errorf("%q is out of order in:\n%s", part, msg)
			}
			pos = i
		}
		if again := func() string { _, e := ResolveTheme(cfg); return e.Error() }(); again != msg {
			t.Error("the error text changed between calls")
		}
	})
}

func TestTag(t *testing.T) {
	var theme Theme
	tests := []struct {
		name  string
		want  string
		color tcell.Color
	}{
		{name: "named color", color: tcell.ColorYellow, want: "[yellow]"},
		{name: "named gray", color: tcell.ColorGray, want: "[gray]"},
		{name: "named dark gray", color: tcell.ColorDarkGray, want: "[darkgray]"},
		{name: "named dark cyan", color: tcell.ColorDarkCyan, want: "[darkcyan]"},
		{name: "named dark magenta", color: tcell.ColorDarkMagenta, want: "[darkmagenta]"},
		{name: "named olive", color: tcell.ColorOlive, want: "[olive]"},
		{name: "named teal", color: tcell.ColorTeal, want: "[teal]"},
		{name: "legacy cyan", color: colorLegacyCyan, want: "[cyan]"},
		{name: "legacy magenta", color: colorLegacyMagenta, want: "[magenta]"},
		{name: "custom color", color: tcell.NewHexColor(0xff8800), want: "[#ff8800]"},
		{name: "low values are zero padded", color: tcell.NewRGBColor(1, 2, 3), want: "[#010203]"},
		{name: "black custom color", color: tcell.NewHexColor(0x000000), want: "[#000000]"},
		{name: "a color named by tcell but not used in tags", color: tcell.ColorAqua, want: "[#00ffff]"},
		{name: "the default color resets", color: tcell.ColorDefault, want: "[-]"},
		{name: "a color without a value resets", color: tcell.PaletteColor(300), want: "[-]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := theme.Tag(tt.color); got != tt.want {
				t.Errorf("Tag(%v) = %q, want %q", tt.color, got, tt.want)
			}
		})
	}
}

// Tag has to produce text that tview reads back as the same color.
func TestTagIsParsedByTview(t *testing.T) {
	var theme Theme
	tests := []struct{ color, want tcell.Color }{
		{tcell.ColorYellow, tcell.ColorYellow},
		{tcell.ColorWhite, tcell.ColorWhite},
		{tcell.ColorRed, tcell.ColorRed},
		{tcell.ColorGreen, tcell.ColorGreen},
		{tcell.ColorGray, tcell.ColorGray},
		{tcell.ColorDarkGray, tcell.ColorDarkGray},
		{tcell.ColorBlue, tcell.ColorBlue},
		{tcell.ColorDarkCyan, tcell.ColorDarkCyan},
		{tcell.ColorDarkMagenta, tcell.ColorDarkMagenta},
		{tcell.ColorOlive, tcell.ColorOlive},
		{tcell.ColorTeal, tcell.ColorTeal},
		{tcell.NewHexColor(0xff8800), tcell.NewHexColor(0xff8800)},
		{tcell.NewHexColor(0x002b36), tcell.NewHexColor(0x002b36)},
		{tcell.ColorAqua, tcell.NewHexColor(0x00ffff)}, // spelled by value, as it has no tag name
	}
	for _, tt := range tests {
		t.Run(theme.Tag(tt.color), func(t *testing.T) {
			view := tview.NewTextView().SetDynamicColors(true).SetText(theme.Tag(tt.color) + "x[-]")

			fg, _ := drawPrimitive(t, view, 10, 1).colorsOf(t, "x")

			if fg != tt.want {
				t.Errorf("rendered color = %v, want %v", fg, tt.want)
			}
		})
	}
}

// tcell does not know the names cyan and magenta, so tview draws text tagged
// with them in the terminal's default foreground. The dark theme keeps those
// two spellings so it looks exactly like the original dashboard; this test
// documents the behavior it depends on and fails if tcell ever changes it, in
// which case the markers in theme.go should become real colors.
func TestLegacyCyanAndMagentaTagsDrawInTheDefaultForeground(t *testing.T) {
	var theme Theme
	for _, color := range []tcell.Color{colorLegacyCyan, colorLegacyMagenta} {
		view := tview.NewTextView().SetDynamicColors(true).SetText(theme.Tag(color) + "x")
		fg, _ := drawPrimitive(t, view, 10, 1).colorsOf(t, "x")
		if fg != tcell.ColorDefault {
			t.Errorf("%s draws in %v, want the default foreground %v", theme.Tag(color), fg, tcell.ColorDefault)
		}
	}
}

func TestApply(t *testing.T) {
	restoreStyles(t)

	light := lightTheme()
	light.Apply()
	got := tview.Styles
	checks := []struct {
		name      string
		have, own tcell.Color
	}{
		{"primitive background", got.PrimitiveBackgroundColor, light.Background},
		{"contrast background", got.ContrastBackgroundColor, light.FieldBg},
		{"border", got.BorderColor, light.Border},
		{"graphics", got.GraphicsColor, light.Border},
		{"title", got.TitleColor, light.Title},
		{"primary text", got.PrimaryTextColor, light.Text},
		{"secondary text", got.SecondaryTextColor, light.Accent},
		{"contrast secondary text", got.ContrastSecondaryTextColor, light.FieldHint},
	}
	for _, c := range checks {
		if c.have != c.own {
			t.Errorf("%s = %v, want %v", c.name, c.have, c.own)
		}
	}

	t.Run("the result does not depend on the previously applied theme", func(t *testing.T) {
		for _, name := range ThemeNames() {
			nordTheme().Apply()
			BuiltinThemes()[name].Apply()
			first := tview.Styles

			solarizedTheme().Apply()
			BuiltinThemes()[name].Apply()
			if tview.Styles != first {
				t.Errorf("applying %s twice, after different themes, gave different styles", name)
			}
		}
	})

	t.Run("the primitives pick the styles up when they are created", func(t *testing.T) {
		solarizedTheme().Apply()
		box := tview.NewBox()
		if box.GetBackgroundColor() != solarizedTheme().Background {
			t.Errorf("box background = %v, want %v", box.GetBackgroundColor(), solarizedTheme().Background)
		}
	})
}

// An unquoted #rrggbb is a YAML comment, so the key ends up with no value. The
// error has to say what went wrong instead of only calling the color invalid.
func TestUnquotedHexColorInYAMLGetsAHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestFile(t, path, "theme:\n  colors:\n    accent: #ff8800\n")
	cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	_, err = ResolveTheme(cfg.Theme)

	if err == nil || !strings.Contains(err.Error(), "theme.colors.accent") || !strings.Contains(err.Error(), "quote #rrggbb") {
		t.Errorf("ResolveTheme() error = %v, want a hint to quote #rrggbb values", err)
	}

	writeTestFile(t, path, "theme:\n  colors:\n    accent: \"#ff8800\"\n")
	cfg, _, err = config.Load(config.LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if theme, err := ResolveTheme(cfg.Theme); err != nil || theme.Accent != tcell.NewHexColor(0xff8800) {
		t.Errorf("the quoted form gave accent %v, error %v, want #ff8800", theme.Accent, err)
	}
}

// The dark theme holds two marker values that are not real tcell colors (see
// colorLegacyCyan). They only mean something to Tag and colorName, so they must
// stay in the roles that are used to build tags and out of every other theme,
// and no other code may hand those roles to tview as a cell or text color.
func TestLegacyMarkersAreOnlyUsedToBuildTags(t *testing.T) {
	isMarker := func(c tcell.Color) bool { return c == colorLegacyCyan || c == colorLegacyMagenta }

	for _, marker := range []tcell.Color{colorLegacyCyan, colorLegacyMagenta} {
		if marker.Valid() || marker.Hex() >= 0 {
			t.Errorf("marker %#x looks like a real color (valid %v, hex %d); it must not", uint64(marker), marker.Valid(), marker.Hex())
		}
	}

	t.Run("only the dark theme has markers, in tag roles", func(t *testing.T) {
		for name, theme := range BuiltinThemes() {
			for role, color := range roleColors(theme) {
				if isMarker(color) && (name != "dark" || role != "subheading") {
					t.Errorf("theme %s role %s holds a legacy marker", name, role)
				}
			}
			for i, color := range theme.ServicePalette {
				if isMarker(color) && (name != "dark" || (i != 1 && i != 2)) {
					t.Errorf("theme %s palette entry %d holds a legacy marker", name, i)
				}
			}
		}
	})

	t.Run("an override replaces a marker with a real color", func(t *testing.T) {
		theme, err := ResolveTheme(config.ThemeConfig{Colors: map[string]string{"subheading": "cyan"}})
		if err != nil || isMarker(theme.Subheading) || !theme.Subheading.Valid() {
			t.Errorf("subheading = %v, error %v, want a real color", theme.Subheading, err)
		}
	})

	t.Run("the rest of the package only builds tags with them", func(t *testing.T) {
		files, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") || file == "theme.go" {
				continue
			}
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				where := fmt.Sprintf("%s:%d: %s", file, i+1, strings.TrimSpace(line))
				switch {
				case strings.Contains(line, "colorLegacy"):
					t.Errorf("only theme.go may use the legacy markers: %s", where)
				case strings.Contains(line, "Subheading") && !strings.Contains(line, "Tag("):
					t.Errorf("Theme.Subheading may be used only inside Tag(): %s", where)
				case strings.Contains(line, "ServicePalette") && !strings.Contains(line, "palette := "):
					t.Errorf("Theme.ServicePalette may only be read into the local palette that colorName turns into a tag: %s", where)
				}
			}
		}
	})
}
