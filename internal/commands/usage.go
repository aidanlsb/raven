package commands

import (
	"fmt"
	"strings"
)

func UsageForMeta(commandID string, meta Meta) string {
	use := strings.TrimSpace(meta.Use)
	if use != "" {
		return use
	}
	return deriveUsageFromArgs(commandID, meta)
}

func FullCLIUsage(commandID string) string {
	meta, ok := EffectiveMeta(commandID)
	if !ok {
		return ""
	}

	use := UsageForMeta(commandID, meta)
	if use == "" {
		return ""
	}

	nameParts := strings.Fields(meta.Name)
	if len(nameParts) == 0 {
		return "rvn " + use
	}

	localName := nameParts[len(nameParts)-1]
	if use == localName ||
		strings.HasPrefix(use, localName+" ") ||
		strings.HasPrefix(use, localName+"<") ||
		strings.HasPrefix(use, localName+"[") {
		if len(nameParts) == 1 {
			return "rvn " + use
		}
		return "rvn " + strings.Join(nameParts[:len(nameParts)-1], " ") + " " + use
	}

	return "rvn " + use
}

func deriveUsageFromArgs(commandID string, meta Meta) string {
	base := strings.ReplaceAll(commandID, "_", " ")
	for _, arg := range meta.Args {
		if arg.Required {
			base += fmt.Sprintf(" <%s>", arg.Name)
		} else {
			base += fmt.Sprintf(" [%s]", arg.Name)
		}
	}
	return base
}
