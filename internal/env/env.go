// Package env resolves environment variables for a Lambda service.
//
// It supports three sources, applied in order (later values win):
//  1. The inline environment map from the .simla config.
//  2. A .env file referenced by envFile in the service config.
//  3. Host environment variables referenced via ${VAR} interpolation.
//
// Values in both the inline map and the .env file may contain ${VAR}
// placeholders, which are expanded against the host's os.Environ().
// Unknown placeholders are left as-is so the Lambda can receive them
// literally (useful for placeholders that will be substituted at runtime
// by another layer).
//
// Secret masking: any key whose name contains SECRET, PASSWORD, TOKEN, or KEY
// (case-insensitive) is masked in log output as "****".
package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns the merged, interpolated environment map for a service.
// base is the inline environment from the config. envFile (if non-empty)
// is a path to a .env file whose values are merged on top.
func Resolve(base map[string]string, envFile string) (map[string]string, error) {
	merged := make(map[string]string, len(base))

	// 1. Start with the inline map.
	for k, v := range base {
		merged[k] = v
	}

	// 2. Overlay values from the .env file.
	if envFile != "" {
		fileVars, err := parseEnvFile(envFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load envFile %q: %w", envFile, err)
		}
		for k, v := range fileVars {
			merged[k] = v
		}
	}

	// 3. Interpolate ${VAR} placeholders against host environment.
	for k, v := range merged {
		merged[k] = interpolate(v)
	}

	return merged, nil
}

// Mask returns a copy of env with sensitive values replaced by "****".
// A key is considered sensitive if it contains SECRET, PASSWORD, TOKEN,
// or KEY (case-insensitive).
func Mask(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if isSensitive(k) {
			out[k] = "****"
		} else {
			out[k] = v
		}
	}
	return out
}

// isSensitive reports whether a key name looks like it holds a secret.
func isSensitive(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// parseEnvFile reads a .env file and returns a map of key→value pairs.
// It supports:
//   - KEY=VALUE
//   - KEY="VALUE WITH SPACES"
//   - KEY='VALUE WITH SPACES'
//   - Lines starting with # are comments and are skipped.
//   - Blank lines are skipped.
//   - export KEY=VALUE (the export prefix is stripped).
func parseEnvFile(path string) (map[string]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip optional "export " prefix.
		line = strings.TrimPrefix(line, "export ")

		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			// No '=' — skip malformed line silently.
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes.
		val = stripQuotes(val)

		if key != "" {
			vars[key] = val
		}
	}

	return vars, scanner.Err()
}

// stripQuotes removes a matching pair of leading/trailing single or double quotes.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// interpolate replaces ${VAR} and $VAR references in s with values from the
// host environment. Unknown references are left as-is.
func interpolate(s string) string {
	return os.Expand(s, func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		// Leave unknown placeholders intact.
		return "${" + key + "}"
	})
}
