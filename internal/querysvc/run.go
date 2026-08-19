package querysvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// IsSectionQueryRoot reports whether a query string targets the section root.
func IsSectionQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "section" || strings.HasPrefix(trimmed, "section ")
}

// IsLinkQueryRoot reports whether a query string targets the link root.
func IsLinkQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return trimmed == "link" || strings.HasPrefix(trimmed, "link ")
}

// IsFullQueryRoot reports whether a query string already starts with a concrete
// query root (type:, trait:, section, or link) rather than a saved-query name.
func IsFullQueryRoot(queryString string) bool {
	trimmed := strings.TrimSpace(queryString)
	return strings.HasPrefix(trimmed, "type:") ||
		strings.HasPrefix(trimmed, "trait:") ||
		IsSectionQueryRoot(trimmed) ||
		IsLinkQueryRoot(trimmed)
}

// MatchInvocation reports whether a raw query string invokes a saved query.
// It returns the saved query name, its definition, and any trailing input
// tokens (positional or key=value) that follow the name. It returns
// matched=false for bare roots, empty strings, or names that are not
// defined as saved queries. This is the single place both the CLI (for run-time
// option merging) and the canonical command handler (for query resolution)
// decide whether an invocation names a saved query.
func MatchInvocation(vaultCfg *config.VaultConfig, rawQueryString string) (name string, saved *config.SavedQuery, inputTokens []string, matched bool) {
	if vaultCfg == nil || len(vaultCfg.Queries) == 0 {
		return "", nil, nil, false
	}

	trimmed := strings.TrimSpace(rawQueryString)
	if trimmed == "" || IsSectionQueryRoot(trimmed) || IsLinkQueryRoot(trimmed) {
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

// RunOptionOverrides holds the runtime flags explicitly supplied for one
// `rvn query` invocation. Saved query definitions never populate these fields.
type RunOptionOverrides struct {
	Refresh   *bool
	IDs       *bool
	Limit     *int
	Offset    *int
	CountOnly *bool
	Apply     []string
	Confirm   *bool
	Pipe      *bool
	Browse    *bool
}

// IsEmpty reports whether no runtime query flags were supplied.
func (o *RunOptionOverrides) IsEmpty() bool {
	if o == nil {
		return true
	}
	return o.Refresh == nil &&
		o.IDs == nil &&
		o.Limit == nil &&
		o.Offset == nil &&
		o.CountOnly == nil &&
		len(o.Apply) == 0 &&
		o.Confirm == nil &&
		o.Pipe == nil &&
		o.Browse == nil
}

// RunOptions holds the effective options and resolved query string for one
// `rvn query` invocation.
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
	// Pipe carries the explicit pipe override (nil means auto-detect).
	Pipe *bool
}

// ResolveRunOptions resolves rawQueryString (expanding a saved-query invocation
// when one is named) and applies only the caller's invocation-scoped flags.
func ResolveRunOptions(rt *vaultruntime.Runtime, rawQueryString string, explicit *RunOptionOverrides) (*RunOptions, error) {
	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}

	name, saved, inputTokens, matched := MatchInvocation(vaultCfg, rawQueryString)
	resolved := rawQueryString
	if matched {
		resolvedQuery, resolveErr := ResolveSavedQuery(name, saved, inputTokens, nil)
		if resolveErr != nil {
			return nil, resolveErr
		}
		resolved = resolvedQuery
	}
	if explicit == nil {
		explicit = &RunOptionOverrides{}
	}

	return &RunOptions{
		ResolvedQuery: resolved,
		IsSaved:       matched,
		SavedName:     name,
		Refresh:       derefBool(explicit.Refresh),
		IDs:           derefBool(explicit.IDs),
		Limit:         derefInt(explicit.Limit),
		Offset:        derefInt(explicit.Offset),
		CountOnly:     derefBool(explicit.CountOnly),
		Apply:         append([]string(nil), explicit.Apply...),
		Confirm:       derefBool(explicit.Confirm),
		Browse:        derefBool(explicit.Browse),
		Pipe:          explicit.Pipe,
	}, nil
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
