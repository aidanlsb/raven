package querysvc

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/workflow"
)

type Code string

const (
	CodeInvalidInput   Code = "INVALID_INPUT"
	CodeQueryInvalid   Code = "QUERY_INVALID"
	CodeQueryNotFound  Code = "QUERY_NOT_FOUND"
	CodeDuplicateName  Code = "DUPLICATE_NAME"
	CodeConfigInvalid  Code = "CONFIG_INVALID"
	CodeFileWriteError Code = "FILE_WRITE_ERROR"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type AddRequest struct {
	VaultPath   string
	Name        string
	QueryString string
	Args        []string
	Description string
}

type AddResult struct {
	Name        string
	Query       string
	Args        []string
	Description string
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

func Add(req AddRequest) (*AddResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newError(CodeInvalidInput, "query name is required", "Usage: rvn query add <name> <query-string>", nil)
	}
	queryStr := strings.TrimSpace(req.QueryString)
	if queryStr == "" {
		return nil, newError(CodeInvalidInput, "query string is required", "Usage: rvn query add <name> <query-string>", nil)
	}

	declaredArgs, err := NormalizeArgs(req.Args)
	if err != nil {
		return nil, err
	}

	if !hasTemplateVars(queryStr) {
		if _, err := query.Parse(queryStr); err != nil {
			return nil, newError(CodeQueryInvalid, fmt.Sprintf("invalid query: %v", err), "", err)
		}
	}
	if err := ValidateInputDeclarations(name, queryStr, declaredArgs); err != nil {
		return nil, err
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", err)
	}
	if _, exists := vaultCfg.Queries[name]; exists {
		return nil, newError(CodeDuplicateName, fmt.Sprintf("query '%s' already exists", name), "Use 'rvn query remove' first to replace it", nil)
	}

	if vaultCfg.Queries == nil {
		vaultCfg.Queries = make(map[string]*config.SavedQuery)
	}
	vaultCfg.Queries[name] = &config.SavedQuery{
		Query:       queryStr,
		Args:        declaredArgs,
		Description: req.Description,
	}

	if err := config.SaveVaultConfig(req.VaultPath, vaultCfg); err != nil {
		return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
	}

	return &AddResult{
		Name:        name,
		Query:       queryStr,
		Args:        declaredArgs,
		Description: req.Description,
	}, nil
}

func Remove(req RemoveRequest) (*RemoveResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, newError(CodeInvalidInput, "query name is required", "Usage: rvn query remove <name>", nil)
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", err)
	}
	if _, exists := vaultCfg.Queries[name]; !exists {
		return nil, newError(CodeQueryNotFound, fmt.Sprintf("query '%s' not found", name), "Run 'rvn query --list' to see available queries", nil)
	}

	delete(vaultCfg.Queries, name)
	if err := config.SaveVaultConfig(req.VaultPath, vaultCfg); err != nil {
		return nil, newError(CodeFileWriteError, "failed to save vault config", "", err)
	}

	return &RemoveResult{Name: name, Removed: true}, nil
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
			return nil, newError(CodeInvalidInput, "saved query has an empty arg name", "Use non-empty arg names, e.g. args: [project]", nil)
		}
		if _, exists := seen[name]; exists {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("saved query declares duplicate arg: %s", name), "Each arg name must be unique", nil)
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
		return nil, newError(
			CodeInvalidInput,
			fmt.Sprintf("saved query '%s' does not declare args", queryName),
			"Declare args in raven.yaml (args: [name, ...]) or remove input arguments",
			nil,
		)
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
				return newError(
					CodeInvalidInput,
					fmt.Sprintf("invalid input argument: %s", arg),
					"Use format: key=value or positional values matching args order",
					nil,
				)
			}
			key := parts[0]
			if _, ok := declaredSet[key]; !ok {
				return newError(
					CodeInvalidInput,
					fmt.Sprintf("unknown input key for saved query '%s': %s", queryName, key),
					fmt.Sprintf("Declared args: %s", strings.Join(declaredArgs, ", ")),
					nil,
				)
			}
			if _, exists := keyValues[key]; exists {
				return newError(
					CodeInvalidInput,
					fmt.Sprintf("duplicate input key: %s", key),
					"Provide each input at most once",
					nil,
				)
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
		return nil, newError(
			CodeInvalidInput,
			fmt.Sprintf("too many positional inputs for saved query '%s' (got %d, expected at most %d)", queryName, len(positional), len(remaining)),
			fmt.Sprintf("Declared args: %s", strings.Join(declaredArgs, ", ")),
			nil,
		)
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
		return nil, newError(CodeInvalidInput, "no apply command specified", "Use --apply <command> [args...]", nil)
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
		return newError(
			CodeInvalidInput,
			fmt.Sprintf("saved query '%s' uses {{args.*}} but does not declare args", name),
			fmt.Sprintf("Declare args in raven.yaml, e.g. args: [%s]", strings.Join(usedInputs, ", ")),
			nil,
		)
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
		return newError(
			CodeInvalidInput,
			fmt.Sprintf("saved query '%s' is missing arg declarations for: %s", name, strings.Join(missing, ", ")),
			fmt.Sprintf("Declare args in raven.yaml, e.g. args: [%s]", strings.Join(usedInputs, ", ")),
			nil,
		)
	}
	return nil
}

func ResolveQueryString(name string, q *config.SavedQuery, inputs map[string]string) (string, error) {
	if q == nil || q.Query == "" {
		return "", newError(CodeQueryInvalid, fmt.Sprintf("saved query '%s' has no query defined", name), "", nil)
	}

	queryStr, err := workflow.Interpolate(normalizeSavedQueryTemplateVars(q.Query), inputs, nil)
	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "inputs.", "args.")
		return "", newError(CodeInvalidInput, fmt.Sprintf("failed to resolve saved query '%s': %s", name, errMsg), "", err)
	}

	return queryStr, nil
}

func ResolveSavedQuery(name string, q *config.SavedQuery, args []string, keyValueArgs []string) (string, error) {
	if q == nil {
		return "", newError(CodeQueryNotFound, fmt.Sprintf("query '%s' not found", name), "Run 'rvn query --list' to see available queries", nil)
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
