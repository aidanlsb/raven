package querysvc

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type SavedQueryInfo struct {
	Name        string
	Query       string
	Args        []string
	Description string
}

// Payload returns the transport-neutral JSON representation of a saved query. It
// is the single shaping shared by the query_saved_list / query_saved_get
// commands and the raven://queries/saved MCP resource, so the CLI, MCP tool, and
// resource views of saved queries stay identical.
func (q SavedQueryInfo) Payload() map[string]interface{} {
	data := map[string]interface{}{
		"name":        q.Name,
		"query":       q.Query,
		"args":        q.Args,
		"description": q.Description,
	}
	return data
}

type ListRequest struct {
	VaultPath string
}

type ListResult struct {
	Queries []SavedQueryInfo
}

type GetRequest struct {
	VaultPath string
	Name      string
}

type GetResult struct {
	Query SavedQueryInfo
}

type SetRequest struct {
	VaultPath   string
	Name        string
	QueryString string
	Args        []string
	Description string
}

type SetStatus string

const (
	SetStatusCreated   SetStatus = "created"
	SetStatusUpdated   SetStatus = "updated"
	SetStatusUnchanged SetStatus = "unchanged"
)

type SetResult struct {
	Query  SavedQueryInfo
	Status SetStatus
}

type RemoveRequest struct {
	VaultPath string
	Name      string
}

type RemoveResult struct {
	Name    string
	Removed bool
}

type ApplyCommand struct {
	Command string
	Args    []string
}

func List(rt *vaultruntime.Runtime, req ListRequest) (*ListResult, error) {
	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(vaultCfg.Queries))
	for name := range vaultCfg.Queries {
		names = append(names, name)
	}
	sort.Strings(names)

	queries := make([]SavedQueryInfo, 0, len(names))
	for _, name := range names {
		queries = append(queries, savedQueryInfo(name, vaultCfg.Queries[name]))
	}

	return &ListResult{Queries: queries}, nil
}

func Get(rt *vaultruntime.Runtime, req GetRequest) (*GetResult, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "query name is required").WithSuggestion("Usage: rvn query saved get <name>")
	}

	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}
	saved, exists := vaultCfg.Queries[name]
	if !exists {
		return nil, svcerr.New(codes.ErrQueryNotFound, fmt.Sprintf("query '%s' not found", name)).WithSuggestion("Run 'rvn query saved list' to see available queries")
	}

	return &GetResult{Query: savedQueryInfo(name, saved)}, nil
}

func Set(rt *vaultruntime.Runtime, req SetRequest) (*SetResult, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "query name is required").WithSuggestion("Usage: rvn query saved set <name> <query-string>")
	}
	queryStr := strings.TrimSpace(req.QueryString)
	if queryStr == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "query string is required").WithSuggestion("Usage: rvn query saved set <name> <query-string>")
	}

	declaredArgs, err := NormalizeArgs(req.Args)
	if err != nil {
		return nil, err
	}

	if !hasTemplateVars(queryStr) {
		if _, err := query.Parse(queryStr); err != nil {
			return nil, svcerr.Wrap(codes.ErrQueryInvalid, fmt.Sprintf("invalid query: %v", err), err)
		}
	}
	if err := ValidateInputDeclarations(name, queryStr, declaredArgs); err != nil {
		return nil, err
	}

	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}

	if vaultCfg.Queries == nil {
		vaultCfg.Queries = make(map[string]*config.SavedQuery)
	}
	next := &config.SavedQuery{
		Query:       queryStr,
		Args:        declaredArgs,
		Description: req.Description,
	}

	status := SetStatusCreated
	if existing, exists := vaultCfg.Queries[name]; exists {
		if savedQueriesEqual(existing, next) {
			return &SetResult{
				Query:  savedQueryInfo(name, existing),
				Status: SetStatusUnchanged,
			}, nil
		}
		status = SetStatusUpdated
	}
	vaultCfg.Queries[name] = next

	if err := saveRuntimeConfig(rt, vaultCfg); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}

	return &SetResult{
		Query:  savedQueryInfo(name, next),
		Status: status,
	}, nil
}

func Remove(rt *vaultruntime.Runtime, req RemoveRequest) (*RemoveResult, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "query name is required").WithSuggestion("Usage: rvn query saved remove <name>")
	}

	vaultCfg, err := runtimeConfig(rt)
	if err != nil {
		return nil, err
	}
	if _, exists := vaultCfg.Queries[name]; !exists {
		return nil, svcerr.New(codes.ErrQueryNotFound, fmt.Sprintf("query '%s' not found", name)).WithSuggestion("Run 'rvn query saved list' to see available queries")
	}

	delete(vaultCfg.Queries, name)
	if err := saveRuntimeConfig(rt, vaultCfg); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}

	return &RemoveResult{Name: name, Removed: true}, nil
}

func runtimeConfig(rt *vaultruntime.Runtime) (*config.VaultConfig, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		if err := rt.ReloadConfig(); err != nil {
			return nil, svcerr.Wrap(codes.ErrConfigInvalid, "failed to load vault config", err).WithSuggestion("Fix raven.yaml and try again")
		}
	}
	return rt.VaultCfg, nil
}

func saveRuntimeConfig(rt *vaultruntime.Runtime, cfg *config.VaultConfig) error {
	err := config.SaveVaultConfig(rt.VaultPath, cfg)
	reloadErr := rt.ReloadConfig()
	if err != nil {
		return err
	}
	return reloadErr
}

func savedQueryInfo(name string, q *config.SavedQuery) SavedQueryInfo {
	if q == nil {
		return SavedQueryInfo{Name: name}
	}
	return SavedQueryInfo{
		Name:        name,
		Query:       q.Query,
		Args:        append([]string(nil), q.Args...),
		Description: q.Description,
	}
}

func savedQueriesEqual(a, b *config.SavedQuery) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Query != b.Query || a.Description != b.Description {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	return true
}

func NormalizeArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(args))
	seen := make(map[string]struct{}, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg)
		if name == "" {
			return nil, svcerr.New(codes.ErrInvalidInput, "saved query has an empty arg name").WithSuggestion("Use non-empty arg names, e.g. args: [project]")
		}
		if _, exists := seen[name]; exists {
			return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("saved query declares duplicate arg: %s", name)).WithSuggestion("Each arg name must be unique")
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized, nil
}

func ParseInputs(queryName string, args []string, declaredArgs []string) (map[string]string, error) {
	return ParseInputsWithKeyValues(queryName, args, nil, declaredArgs)
}

func ParseInputsWithKeyValues(queryName string, args []string, keyValueArgs []string, declaredArgs []string) (map[string]string, error) {
	if len(args) == 0 && len(keyValueArgs) == 0 {
		return nil, nil
	}

	if len(declaredArgs) == 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("saved query '%s' does not declare args", queryName)).WithSuggestion("Declare args in raven.yaml (args: [name, ...]) or remove input arguments")
	}

	declaredSet := make(map[string]struct{}, len(declaredArgs))
	for _, name := range declaredArgs {
		declaredSet[name] = struct{}{}
	}

	keyValues := make(map[string]string, len(args)+len(keyValueArgs))
	positional := make([]string, 0, len(args))
	parseToken := func(arg string) error {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 || parts[0] == "" {
				return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid input argument: %s", arg)).WithSuggestion("Use format: key=value or positional values matching args order")
			}
			key := parts[0]
			if _, ok := declaredSet[key]; !ok {
				return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("unknown input key for saved query '%s': %s", queryName, key)).WithSuggestion(fmt.Sprintf("Declared args: %s", strings.Join(declaredArgs, ", ")))
			}
			if _, exists := keyValues[key]; exists {
				return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("duplicate input key: %s", key)).WithSuggestion("Provide each input at most once")
			}
			keyValues[key] = parts[1]
			return nil
		}
		positional = append(positional, arg)
		return nil
	}

	for _, arg := range args {
		if err := parseToken(arg); err != nil {
			return nil, err
		}
	}
	for _, arg := range keyValueArgs {
		if err := parseToken(arg); err != nil {
			return nil, err
		}
	}

	remaining := make([]string, 0, len(declaredArgs))
	for _, name := range declaredArgs {
		if _, provided := keyValues[name]; !provided {
			remaining = append(remaining, name)
		}
	}

	if len(positional) > len(remaining) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("too many positional inputs for saved query '%s' (got %d, expected at most %d)", queryName, len(positional), len(remaining))).WithSuggestion(fmt.Sprintf("Declared args: %s", strings.Join(declaredArgs, ", ")))
	}

	inputs := make(map[string]string, len(keyValues)+len(positional))
	for k, v := range keyValues {
		inputs[k] = v
	}
	for i, v := range positional {
		inputs[remaining[i]] = v
	}
	return inputs, nil
}

func ParseApplyCommand(applyArgs []string) (*ApplyCommand, error) {
	applyStr := strings.Join(applyArgs, " ")
	parts := strings.Fields(applyStr)
	if len(parts) == 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "no apply command specified").WithSuggestion("Use --apply <command> [args...]")
	}
	return &ApplyCommand{
		Command: parts[0],
		Args:    parts[1:],
	}, nil
}

var savedQueryInputRefPattern = regexp.MustCompile(`\{\{\s*(args|inputs)\.([A-Za-z0-9_-]+)\s*\}\}`)
var savedQueryArgsRefPattern = regexp.MustCompile(`\{\{\s*args\.([A-Za-z0-9_-]+)\s*\}\}`)

func ValidateInputDeclarations(name, queryStr string, declaredArgs []string) error {
	usedInputs := extractSavedQueryInputRefs(queryStr)
	if len(usedInputs) == 0 {
		return nil
	}
	if len(declaredArgs) == 0 {
		return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("saved query '%s' uses {{args.*}} but does not declare args", name)).WithSuggestion(fmt.Sprintf("Declare args in raven.yaml, e.g. args: [%s]", strings.Join(usedInputs, ", ")))
	}

	declaredSet := make(map[string]struct{}, len(declaredArgs))
	for _, arg := range declaredArgs {
		declaredSet[arg] = struct{}{}
	}

	missing := make([]string, 0)
	for _, input := range usedInputs {
		if _, ok := declaredSet[input]; !ok {
			missing = append(missing, input)
		}
	}
	if len(missing) > 0 {
		return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("saved query '%s' is missing arg declarations for: %s", name, strings.Join(missing, ", "))).WithSuggestion(fmt.Sprintf("Declare args in raven.yaml, e.g. args: [%s]", strings.Join(usedInputs, ", ")))
	}
	return nil
}

func ResolveQueryString(name string, q *config.SavedQuery, inputs map[string]string) (string, error) {
	if q == nil || q.Query == "" {
		return "", svcerr.New(codes.ErrQueryInvalid, fmt.Sprintf("saved query '%s' has no query defined", name))
	}

	queryStr, err := interpolateSavedQueryInputs(normalizeSavedQueryTemplateVars(q.Query), inputs)
	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "inputs.", "args.")
		return "", svcerr.Wrap(codes.ErrInvalidInput, fmt.Sprintf("failed to resolve saved query '%s': %s", name, errMsg), err)
	}

	return queryStr, nil
}

func ResolveSavedQuery(name string, q *config.SavedQuery, args []string, keyValueArgs []string) (string, error) {
	if q == nil {
		return "", svcerr.New(codes.ErrQueryNotFound, fmt.Sprintf("query '%s' not found", name)).WithSuggestion("Run 'rvn query saved list' to see available queries")
	}

	declaredArgs, err := NormalizeArgs(q.Args)
	if err != nil {
		return "", err
	}
	if err := ValidateInputDeclarations(name, q.Query, declaredArgs); err != nil {
		return "", err
	}

	inputs, err := ParseInputsWithKeyValues(name, args, keyValueArgs, declaredArgs)
	if err != nil {
		return "", err
	}

	return ResolveQueryString(name, q, inputs)
}

func extractSavedQueryInputRefs(queryStr string) []string {
	if queryStr == "" {
		return nil
	}

	seen := make(map[string]struct{})
	inputs := make([]string, 0)
	for _, match := range savedQueryInputRefPattern.FindAllStringSubmatchIndex(queryStr, -1) {
		if len(match) < 6 {
			continue
		}
		start := match[0]
		if start > 0 && queryStr[start-1] == '\\' {
			continue
		}
		name := queryStr[match[4]:match[5]]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		inputs = append(inputs, name)
	}
	return inputs
}

func normalizeSavedQueryTemplateVars(queryStr string) string {
	if queryStr == "" {
		return queryStr
	}

	matches := savedQueryArgsRefPattern.FindAllStringSubmatchIndex(queryStr, -1)
	if len(matches) == 0 {
		return queryStr
	}

	var b strings.Builder
	b.Grow(len(queryStr))
	last := 0
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		start := m[0]
		end := m[1]
		if start > 0 && queryStr[start-1] == '\\' {
			continue
		}

		argName := queryStr[m[2]:m[3]]
		b.WriteString(queryStr[last:start])
		b.WriteString("{{inputs.")
		b.WriteString(argName)
		b.WriteString("}}")
		last = end
	}

	if last == 0 {
		return queryStr
	}
	b.WriteString(queryStr[last:])
	return b.String()
}

func hasTemplateVars(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

var savedQueryInterpolationPattern = regexp.MustCompile(`\{\{\s*inputs\.([A-Za-z0-9_-]+)\s*\}\}`)

func interpolateSavedQueryInputs(queryStr string, inputs map[string]string) (string, error) {
	if queryStr == "" {
		return queryStr, nil
	}

	var b strings.Builder
	b.Grow(len(queryStr))
	last := 0

	for _, match := range savedQueryInterpolationPattern.FindAllStringSubmatchIndex(queryStr, -1) {
		if len(match) < 4 {
			continue
		}
		start := match[0]
		end := match[1]
		if start > 0 && queryStr[start-1] == '\\' {
			continue
		}

		name := queryStr[match[2]:match[3]]
		value, ok := inputs[name]
		if !ok {
			return "", fmt.Errorf("unknown variable: inputs.%s", name)
		}

		b.WriteString(queryStr[last:start])
		b.WriteString(value)
		last = end
	}

	if last == 0 {
		return queryStr, nil
	}

	b.WriteString(queryStr[last:])
	return b.String(), nil
}
