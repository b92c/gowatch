package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/ui"
)

// A keybindings problem stops the start-up before the Docker client and the
// dashboard are created, with a message that names the setting.
func TestRunKeybindingErrorsStopStartup(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "emacs is not a preset",
			content: "keybindings:\n  preset: emacs\n",
			want:    []string{"keybindings.preset", `"emacs"`, "default, vim"},
		},
		{
			name:    "unknown action",
			content: "keybindings:\n  bindings:\n    qiut: [x]\n",
			want:    []string{"keybindings.bindings.qiut", "unknown action", "quit", "scroll_bottom"},
		},
		{
			name:    "invalid key",
			content: "keybindings:\n  bindings:\n    quit: [\"ctrl+shift+q\"]\n",
			want:    []string{"keybindings.bindings.quit", `"ctrl+shift+q"`, "ctrl+<letter>", "pgdn"},
		},
		{
			name:    "ctrl+c is reserved",
			content: "keybindings:\n  bindings:\n    quit: [q, ctrl+c]\n",
			want:    []string{"keybindings.bindings.quit", "ctrl+c", "reserved", "always quits"},
		},
		{
			name:    "an action with no keys",
			content: "keybindings:\n  bindings:\n    quit: []\n",
			want:    []string{"keybindings.bindings.quit", "at least one key"},
		},
		{
			name:    "one key on two actions",
			content: "keybindings:\n  bindings:\n    quit: [x]\n    search: [x]\n",
			want:    []string{"keybindings:", `"x"`, `"quit" and "search"`},
		},
		{
			name:    "the same key twice in an action",
			content: "keybindings:\n  bindings:\n    quit: [x, x]\n",
			want:    []string{"keybindings.bindings.quit", `"x"`, "more than once"},
		},
		{
			name:    "a key of the preset taken by an override",
			content: "keybindings:\n  preset: vim\n  bindings:\n    help: [j]\n",
			want:    []string{`"j"`, `"help" and "scroll_down"`},
		},
		{
			name:    "several problems are all reported",
			content: "keybindings:\n  preset: emacs\n  bindings:\n    nope: [x]\n    quit: [ctrl+c]\n",
			want:    []string{"keybindings.preset", "keybindings.bindings.nope", "keybindings.bindings.quit"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run([]string{"--config", writeConfig(t, dir, "keys.yaml", tt.content)}, &stdout, &stderr)

			if code != 1 {
				t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.HasPrefix(stderr.String(), "gowatch: keybindings") {
				t.Errorf("stderr should start with \"gowatch: keybindings\", got %q", stderr.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
		})
	}
}

// Valid settings get past the keybindings check. The config is loaded and
// turned into a keymap as run does it, because running it for real would start
// the dashboard.
func TestValidKeybindingsLoadIntoAKeymap(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
	}{
		{"no keybindings", "refresh_interval: 2s\n"},
		{"the default preset", "keybindings:\n  preset: default\n"},
		{"the vim preset", "keybindings:\n  preset: vim\n"},
		{"an override", "keybindings:\n  bindings:\n    quit: [\"q\", \"x\"]\n"},
		{"quoted special characters", "keybindings:\n  bindings:\n    help: [\"?\", \"#\"]\n    search: [\"/\", \",\", \"[\"]\n    docker_view: [\"{\", \":\"]\n"},
		{"vim with an override", "keybindings:\n  preset: vim\n  bindings:\n    scroll_down: [\"j\", \"down\", \"ctrl+n\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: writeConfig(t, dir, "ok.yaml", tt.content)})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if _, err := ui.BuildKeymap(cfg.Keybindings); err != nil {
				t.Errorf("BuildKeymap() error = %v", err)
			}
		})
	}
}

// The layered config files merge the bindings map by action, and the later
// file wins for an action both set.
func TestKeybindingsLayerAcrossConfigFiles(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeConfig(t, home, ".gowatch/config.yaml", "keybindings:\n  bindings:\n    quit: [x]\n    help: [h]\n")
	writeConfig(t, work, "gowatch.yaml", "keybindings:\n  bindings:\n    quit: [z]\n")

	cfg, _, err := config.Load(config.LoadOptions{HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	km, err := ui.BuildKeymap(cfg.Keybindings)
	if err != nil {
		t.Fatal(err)
	}
	keys := func(a ui.Action) string {
		var out []string
		for _, k := range km.Keys(a) {
			out = append(out, k.String())
		}
		return strings.Join(out, ",")
	}
	if got := keys(ui.ActionQuit); got != "z" {
		t.Errorf("quit keys = %q, want the working directory file's z", got)
	}
	if got := keys(ui.ActionHelp); got != "h" {
		t.Errorf("help keys = %q, want the home file's h", got)
	}
}

// The printed example documents the keybindings, loads strictly, and turns into
// the default keymap: nothing changes when it is used as the config file.
func TestPrintedDefaultConfigCoversKeybindings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--print-default-config"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	printed := stdout.String()

	for _, want := range append(ui.ActionNames(), ui.PresetNames()...) {
		if !strings.Contains(printed, want) {
			t.Errorf("--print-default-config does not mention %q", want)
		}
	}

	cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: writeConfig(t, t.TempDir(), "printed.yaml", printed)})
	if err != nil {
		t.Fatalf("the printed configuration does not load: %v", err)
	}
	km, err := ui.BuildKeymap(cfg.Keybindings)
	if err != nil {
		t.Fatalf("the printed keybindings are not valid: %v", err)
	}
	want := ui.DefaultKeymap()
	for _, action := range ui.Actions() {
		if got, w := keySpellings(km, action), keySpellings(want, action); got != w {
			t.Errorf("%s keys from the printed config = %q, want the default %q", action, got, w)
		}
	}

	t.Run("the documented default keys are the default preset's", func(t *testing.T) {
		for _, action := range ui.Actions() {
			line := regexp.MustCompile(`(?m)^#   ` + string(action) + ` +(\S.*?)  +\S`).FindStringSubmatch(printed)
			if line == nil {
				t.Errorf("--print-default-config has no line for the action %s", action)
				continue
			}
			if got, w := strings.ReplaceAll(line[1], " ", ""), keySpellings(want, action); got != w {
				t.Errorf("the documented keys of %s are %q, the default preset has %q", action, got, w)
			}
		}
	})

	t.Run("the commented example works once uncommented", func(t *testing.T) {
		example := regexp.MustCompile(`(?m)^  # (bindings:|  [a-z_]+: \[.*\])$`)
		matches := example.FindAllString(printed, -1)
		if len(matches) < 3 {
			t.Fatalf("found %d lines of the commented bindings example, want at least 3", len(matches))
		}
		uncommented := strings.Replace(printed, strings.Join(matches, "\n"), strings.ReplaceAll(strings.Join(matches, "\n"), "  # ", "  "), 1)
		if uncommented == printed {
			t.Fatal("the example lines are not one block")
		}

		cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: writeConfig(t, t.TempDir(), "example.yaml", uncommented)})
		if err != nil {
			t.Fatalf("the uncommented example does not load: %v\n%s", err, uncommented)
		}
		if len(cfg.Keybindings.Bindings) == 0 {
			t.Fatal("uncommenting the example did not set any binding")
		}
		if _, err := ui.BuildKeymap(cfg.Keybindings); err != nil {
			t.Errorf("the example bindings are not valid: %v", err)
		}
	})
}

func keySpellings(km ui.Keymap, action ui.Action) string {
	var out []string
	for _, k := range km.Keys(action) {
		out = append(out, k.String())
	}
	return strings.Join(out, ",")
}
