package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// LoadOptions tells Load where to look for configuration files. The
// directories are injectable so tests can use temporary directories.
type LoadOptions struct {
	// ExplicitPath, when set, is the only file read besides the built-in
	// defaults. It must exist.
	ExplicitPath string
	// HomeDir is the directory that holds .gowatch/config.yaml. Empty skips
	// that layer.
	HomeDir string
	// WorkDir is the directory that holds gowatch.yaml. Empty skips that layer.
	// A working directory can be an untrusted checkout, so this layer may not
	// set aws.endpoint_url, a non-loopback tracing.otlp_receiver.address or
	// tracing.otlp_receiver.enabled: true; Load rejects the file if it does.
	WorkDir string
}

// Load builds the configuration from the built-in defaults and the config files,
// in increasing priority: ~/.gowatch/config.yaml, then ./gowatch.yaml. With
// ExplicitPath only that file is read. Each file is decoded over the result of
// the previous layer, so a key a file leaves out keeps its earlier value.
// Lists are replaced as a whole, maps are merged by key.
//
// Missing files in the default locations are skipped. Unknown keys, malformed
// YAML and invalid values are errors. Load returns the files it read, in the
// order they were applied.
func Load(opts LoadOptions) (*Config, []string, error) {
	cfg := Default()

	var sources []source
	if opts.ExplicitPath != "" {
		sources = []source{{path: opts.ExplicitPath, required: true}}
	} else {
		if opts.HomeDir != "" {
			sources = append(sources, source{path: filepath.Join(opts.HomeDir, ".gowatch", "config.yaml")})
		}
		if opts.WorkDir != "" {
			sources = append(sources, source{path: filepath.Join(opts.WorkDir, "gowatch.yaml"), untrusted: true})
		}
	}

	var loaded []string
	for _, src := range sources {
		found, err := cfg.applyFile(src)
		if err != nil {
			return nil, nil, err
		}
		if found {
			loaded = append(loaded, filepath.Clean(src.path))
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration (%s):\n%w", describeSources(loaded), err)
	}
	return &cfg, loaded, nil
}

// source is one config file layer.
type source struct {
	path string
	// required makes a missing file an error (the --config file).
	required bool
	// untrusted marks the working-directory file, which may come from a cloned
	// repository and so may not set the values in untrustedSettings.
	untrusted bool
}

// applyFile decodes one config file over c. It reports whether the file
// existed; a missing file is only an error when the source is required.
func (c *Config) applyFile(src source) (bool, error) {
	// The path is chosen by the local user (--config flag or the fixed default
	// locations), so it is cleaned but not restricted to a directory.
	clean := filepath.Clean(src.path)
	data, err := os.ReadFile(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !src.required {
			return false, nil
		}
		return false, fmt.Errorf("reading config file: %w", err)
	}

	if src.untrusted {
		// Decode into a fresh Config first: only what this file itself sets is
		// non-zero there, whatever earlier layers contributed.
		var probe Config
		if err := decodeStrict(clean, data, &probe); err != nil {
			return true, err
		}
		if err := probe.untrustedSettings(clean); err != nil {
			return true, err
		}
	}
	return true, decodeStrict(clean, data, c)
}

// decodeStrict decodes the single YAML document in data into c, rejecting
// unknown keys and any further document.
func decodeStrict(path string, data []byte, c *Config) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		if errors.Is(err, io.EOF) { // empty or comment-only file
			return nil
		}
		return decodeError(path, err)
	}

	// Without this check a second document would be silently ignored, typos
	// included.
	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return decodeError(path, err)
	case isEmptyDocument(&extra):
		return nil
	default:
		return fmt.Errorf("%s: line %d: only one YAML document per file is supported, remove the extra \"---\" section", path, firstLine(&extra))
	}
}

// isEmptyDocument reports whether n is a document with no content, such as a
// trailing "---".
func isEmptyDocument(n *yaml.Node) bool {
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 {
		return true
	}
	body := n.Content[0]
	return body.Kind == yaml.ScalarNode && body.Tag == "!!null"
}

func firstLine(n *yaml.Node) int {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0].Line
	}
	return n.Line
}

// decodeError prefixes every YAML problem with the file it came from. The YAML
// library already includes the line number in each message.
func decodeError(path string, err error) error {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		errs := make([]error, 0, len(typeErr.Errors))
		for _, msg := range typeErr.Errors {
			errs = append(errs, fmt.Errorf("%s: %s", path, msg))
		}
		return errors.Join(errs...)
	}
	return fmt.Errorf("%s: %w", path, err)
}

func describeSources(files []string) string {
	if len(files) == 0 {
		return "built-in defaults"
	}
	return strings.Join(files, ", ")
}
