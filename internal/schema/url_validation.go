package schema

import (
	"fmt"
	neturl "net/url"
	"strings"
)

func validateURLString(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("expected URL")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("invalid URL format")
	}

	parsed, err := neturl.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("URL must include a scheme (e.g., https://)")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return fmt.Errorf("URL must include a host")
		}
	default:
		if parsed.Host == "" && parsed.Opaque == "" && parsed.Path == "" {
			return fmt.Errorf("URL is missing a target")
		}
	}

	return nil
}
