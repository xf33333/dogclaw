package constants

import "os"

// GetGrowthBookClientKey returns the GrowthBook client key based on environment.
func GetGrowthBookClientKey() string {
	userType := os.Getenv("USER_TYPE")
	if userType == "ant" {
		if isEnvTruthyGrowthBook(os.Getenv("ENABLE_GROWTHBOOK_DEV")) {
			return "sdk-yZQvlplybuXjYh6L"
		}
		return "sdk-xRVcrliHIlrg4og4"
	}
	return "sdk-zAZezfDKGoZuXXKe"
}

func isEnvTruthyGrowthBook(v string) bool {
	return v == "1" || v == "true" || v == "yes"
}
