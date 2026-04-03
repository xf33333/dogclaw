package utils

// IsEnvTruthy checks if an environment variable value is truthy.
// Returns true for: "1", "true", "yes", "on" (case-insensitive).
func IsEnvTruthy(v string) bool {
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
