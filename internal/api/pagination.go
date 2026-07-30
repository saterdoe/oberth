package api

import "strconv"

func parseIntParam(raw string, fallback int) int {
	if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
		return value
	}
	return fallback
}
