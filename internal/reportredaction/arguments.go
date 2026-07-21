package reportredaction

import "strings"

type redactedError struct {
	message string
	cause   error
}

type replacement struct{ raw, safe string }

func (err redactedError) Error() string { return err.message }
func (err redactedError) Unwrap() error { return err.cause }

// EnvironmentArguments returns a report-safe copy of command arguments.
// Values passed through the conventional --env/-e forms are never disclosed.
func EnvironmentArguments(args []string) []string {
	result := append([]string(nil), args...)
	for i := range result {
		if result[i] == "--env" || result[i] == "-e" {
			if i+1 < len(result) {
				result[i+1] = environmentValue(result[i+1])
				i++
			}
			continue
		}
		for _, prefix := range []string{"--env=", "-e="} {
			if strings.HasPrefix(result[i], prefix) {
				result[i] = prefix + environmentValue(strings.TrimPrefix(result[i], prefix))
			}
		}
	}
	return result
}

func environmentValue(value string) string {
	if key, _, ok := strings.Cut(value, "="); ok {
		return key + "=<redacted>"
	}
	return "<redacted>"
}

// Error returns an error whose printable form excludes environment argument
// values and other sealed payloads while preserving errors.Is/As behavior.
func Error(err error, argumentSets [][]string, sealedPayloads []string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, args := range argumentSets {
		for _, value := range environmentValues(args) {
			message = strings.ReplaceAll(message, value.raw, value.safe)
		}
	}
	for _, payload := range sealedPayloads {
		if payload != "" {
			message = strings.ReplaceAll(message, payload, "<redacted>")
		}
	}
	return redactedError{message: message, cause: err}
}

func environmentValues(args []string) []replacement {
	values := make([]replacement, 0)
	for i := range args {
		if args[i] == "--env" || args[i] == "-e" {
			if i+1 < len(args) {
				values = append(values, replacement{raw: args[i+1], safe: environmentValue(args[i+1])})
				i++
			}
			continue
		}
		for _, prefix := range []string{"--env=", "-e="} {
			if strings.HasPrefix(args[i], prefix) {
				raw := strings.TrimPrefix(args[i], prefix)
				values = append(values, replacement{raw: raw, safe: environmentValue(raw)})
			}
		}
	}
	return values
}
