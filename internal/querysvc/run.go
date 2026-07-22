package querysvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// IsAssetQueryRoot reports whether a query string targets the asset root.
func IsAssetQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "asset" || strings.HasPrefix(trimmed, "asset ")
}

// IsSectionQueryRoot reports whether a query string targets the section root.
func IsSectionQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "section" || strings.HasPrefix(trimmed, "section ")
}

// IsFullQueryRoot reports whether a query string already starts with a concrete
// query root (type:, trait:, section, or asset) rather than a saved-query name.
func IsFullQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return strings.HasPrefix(trimmed, "type:") ||
		strings.HasPrefix(trimmed, "trait:") ||
		IsAssetQueryRoot(trimmed) ||
		IsSectionQueryRoot(trimmed)
}

// MatchInvocation reports whether a raw query string invokes a saved query.
// It returns the saved query name, its definition, and any trailing input
// tokens (positional or key=value) that follow the name. It returns
// matched=false for asset/section roots, empty strings, or names that are not
// defined as saved queries. This is the single place both the CLI (for run-time
// option merging) and the canonical command handler (for query resolution)
// decide whether an invocation names a saved query.
func MatchInvocation(vaultCfg *config.VaultConfig, rawQueryString string) (name string, saved *config.SavedQuery, inputTokens []string, matched bool) {
	if vaultCfg == nil || len(vaultCfg.Queries) == 0 {
		return "", nil, nil, false
	}

	trimmed := strings.TrimSpace(rawQueryString)
	if trimmed == "" || IsAssetQueryRoot(trimmed) || IsSectionQueryRoot(trimmed) {
		return "", nil, nil, false
	}

	var tokens []string
	if strings.ContainsAny(trimmed, " \t\r\n") {
		if parts, ok := SplitInlineInvocation(trimmed); ok {
			tokens = parts
		} else {
			tokens = strings.Fields(trimmed)
		}
	} else {
		tokens = []string{trimmed}
	}
	if len(tokens) == 0 {
		return "", nil, nil, false
	}

	q, ok := vaultCfg.Queries[tokens[0]]
	if !ok {
		return "", nil, nil, false
	}
	return tokens[0], q, tokens[1:], true
}

// RunOptions holds the effective options for a `rvn query` invocation after
// merging explicit flags over a saved query's stored defaults, plus the
// resolved query string used for rendering.
type RunOptions struct {
	// ResolvedQuery is the executable query string. For a saved query it is the
	// interpolated query; otherwise it is the raw input unchanged.
	ResolvedQuery string
	// IsSaved reports whether the invocation named a saved query.
	IsSaved bool
	// SavedName is the saved query name when IsSaved is true.
	SavedName string

	Refresh   bool
	IDs       bool
	Limit     int
	Offset    int
	CountOnly bool
	Apply     []string
	Confirm   bool
	Browse    bool
	// Pipe carries the explicit/saved pipe override (nil means auto-detect).
	Pipe *bool
}

// ResolveRunOptions resolves rawQueryString (expanding a saved-query invocation
// when one is named), and merges the caller's explicit
// flag values over the saved query's stored option defaults. Explicit values
// always win; unset explicit fields fall back to saved defaults, then to zero
// values.
//
// It exists so saved-query resolution and option merging live in one shared
// service rather than being reimplemented in the CLI.
func ResolveRunOptions(rt *vaultruntime.Runtime, rawQueryString string, explicit *config.QueryOptions) (*RunOptions, error) {
	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}

	name, saved, inputTokens, matched := MatchInvocation(vaultCfg, rawQueryString)
	resolved := rawQueryString
	var savedOpts *config.QueryOptions
	if matched {
		resolvedQuery, resolveErr := ResolveSavedQuery(name, saved, inputTokens, nil)
		if resolveErr != nil {
			return nil, resolveErr
		}
		resolved = resolvedQuery
		savedOpts = saved.Options
	}

	merged := mergeRunOptions(explicit, savedOpts)
	return &RunOptions{
		ResolvedQuery: resolved,
		IsSaved:       matched,
		SavedName:     name,
		Refresh:       derefBool(merged.Refresh),
		IDs:           derefBool(merged.IDs),
		Limit:         derefInt(merged.Limit),
		Offset:        derefInt(merged.Offset),
		CountOnly:     derefBool(merged.CountOnly),
		Apply:         merged.Apply,
		Confirm:       derefBool(merged.Confirm),
		Browse:        derefBool(merged.Browse),
		Pipe:          merged.Pipe,
	}, nil
}

// mergeRunOptions overlays explicit option values on top of saved defaults,
// returning a combined options set. Only fields present in explicit override the
// saved value.
func mergeRunOptions(explicit, saved *config.QueryOptions) *config.QueryOptions {
	merged := &config.QueryOptions{}
	if saved != nil {
		*merged = *saved
		merged.Apply = append([]string(nil), saved.Apply...)
	}
	if explicit != nil {
		if explicit.Refresh != nil {
			merged.Refresh = explicit.Refresh
		}
		if explicit.IDs != nil {
			merged.IDs = explicit.IDs
		}
		if explicit.Limit != nil {
			merged.Limit = explicit.Limit
		}
		if explicit.Offset != nil {
			merged.Offset = explicit.Offset
		}
		if explicit.CountOnly != nil {
			merged.CountOnly = explicit.CountOnly
		}
		if explicit.Apply != nil {
			merged.Apply = append([]string(nil), explicit.Apply...)
		}
		if explicit.Confirm != nil {
			merged.Confirm = explicit.Confirm
		}
		if explicit.Pipe != nil {
			merged.Pipe = explicit.Pipe
		}
		if explicit.Browse != nil {
			merged.Browse = explicit.Browse
		}
	}
	return merged
}

func derefBool(v *bool) bool {
	return v != nil && *v
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
