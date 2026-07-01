package check

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
)

// MissingRef represents a reference to a non-existent object.
type MissingRef struct {
	TargetPath     string          // The reference path (e.g., "people/baldur")
	SourceFile     string          // File containing the reference
	SourceObjectID string          // Full object ID where ref was found (e.g., "daily/2026-01-01#team-sync")
	Line           int             // Line number
	InferredType   string          // Type inferred from context (empty if unknown)
	Confidence     InferConfidence // How confident we are about the type
	FieldSource    string          // If from a typed field, the field name (e.g., "attendees")
}

// InferConfidence indicates how confident we are about type inference.
type InferConfidence int

const (
	ConfidenceUnknown  InferConfidence = iota // No type inference possible
	ConfidenceInferred                        // Inferred from path matching default_path
	ConfidenceCertain                         // Certain from typed field
)

func (c InferConfidence) String() string {
	switch c {
	case ConfidenceCertain:
		return "certain"
	case ConfidenceInferred:
		return "inferred"
	default:
		return "unknown"
	}
}

// inferTypeFromPath tries to match a path to a type's default_path.
func (v *Validator) inferTypeFromPath(targetPath string) (typeName string, confidence InferConfidence) {
	if v.isDailyNotePath(targetPath) {
		return "date", ConfidenceInferred
	}

	for name, typeDef := range v.schema.Types {
		if typeDef.DefaultPath != "" {
			// Check if path starts with default_path
			if len(targetPath) > len(typeDef.DefaultPath) &&
				targetPath[:len(typeDef.DefaultPath)] == typeDef.DefaultPath {
				return name, ConfidenceInferred
			}
		}
	}
	return "", ConfidenceUnknown
}

func (v *Validator) isDailyNotePath(targetPath string) bool {
	dailyRoot := paths.NormalizeDirRoot(v.dailyDir)
	if dailyRoot == "" {
		return false
	}

	baseID, _, _ := paths.ParseSectionID(paths.NormalizeVaultRelPath(targetPath))
	baseID = paths.TrimMDExtension(baseID)
	if !strings.HasPrefix(baseID, dailyRoot) {
		return false
	}

	datePart := strings.TrimPrefix(baseID, dailyRoot)
	return !strings.Contains(datePart, "/") && dates.IsValidDate(datePart)
}

func (v *Validator) validateRef(filePath string, ref *parser.ParsedRef) []Issue {
	return v.validateRefWithContext(filePath, "", ref, "", "")
}

// validateRefWithContext validates a reference with optional type context.
// If targetType is provided (from a typed field), we have certain confidence about the type.
func (v *Validator) validateRefWithContext(filePath, sourceObjectID string, ref *parser.ParsedRef, targetType, fieldName string) []Issue {
	var issues []Issue

	if ref == nil || ref.TargetRaw == "" {
		return issues
	}
	if strings.HasPrefix(ref.TargetRaw, "#") {
		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     IssueLocalFragmentRef,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  fmt.Sprintf("Local fragment reference [[%s]] is not supported", ref.TargetRaw),
			Value:    ref.TargetRaw,
			FixHint:  "Use a global section reference like [[object#fragment]]",
		})
		return issues
	}

	// Track short name usage (references without path separators)
	// This is used to only warn about collisions for short names that are actually used
	if !strings.Contains(ref.TargetRaw, "/") && !strings.HasPrefix(ref.TargetRaw, "#") {
		v.usedShortNames[ref.TargetRaw] = struct{}{}
	}

	result := v.resolver.Resolve(ref.TargetRaw)

	if result.Ambiguous {
		message := formatAmbiguousRefMessage(ref.TargetRaw, result, v.displayID)
		fixHint := formatAmbiguousRefFixHint(result, v.displayID)
		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     IssueAmbiguousReference,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  message,
			Value:    ref.TargetRaw,
			FixHint:  fixHint,
		})
	} else if result.TargetID == "" {
		// Check if this is a stale fragment reference (file exists but section doesn't)
		if baseID, fragment, isSection := paths.ParseSectionID(ref.TargetRaw); isSection && fragment != "" {
			baseResult := v.resolver.Resolve(baseID)
			if baseResult.TargetID != "" {
				issues = append(issues, Issue{
					Level:    LevelWarning,
					Type:     IssueStaleFragment,
					FilePath: filePath,
					Line:     ref.Line,
					Message:  fmt.Sprintf("Fragment reference [[%s]] not found — '%s' exists but has no section '#%s'", ref.TargetRaw, v.displayID(baseResult.TargetID), fragment),
					Value:    ref.TargetRaw,
					FixHint:  "The heading may have been renamed. Update the fragment to match an existing section slug.",
				})
				return issues
			}
		}

		// Determine the fix command based on type inference
		fixCmd := ""
		fixHint := ""
		if targetType != "" {
			fixCmd = fmt.Sprintf("rvn new %s \"%s\"", targetType, ref.TargetRaw)
			fixHint = fmt.Sprintf("Create the missing %s", targetType)
		} else {
			// Try to infer from path
			inferredType, conf := v.inferTypeFromPath(ref.TargetRaw)
			if conf == ConfidenceInferred {
				fixCmd = fmt.Sprintf("rvn new %s \"%s\"", inferredType, ref.TargetRaw)
				fixHint = fmt.Sprintf("Create the missing %s (inferred from path)", inferredType)
			} else {
				fixHint = "Create the missing page with 'rvn new <type> <title>'"
			}
		}

		if looksLikeAssetReference(ref.TargetRaw) && targetType == "" {
			issues = append(issues, Issue{
				Level:    LevelError,
				Type:     IssueMissingAsset,
				FilePath: filePath,
				Line:     ref.Line,
				Message:  fmt.Sprintf("Asset reference %q not found", ref.TargetRaw),
				Value:    ref.TargetRaw,
				FixHint:  "Add the asset file under the configured assets root or update the Markdown link",
			})
			return issues
		}

		issues = append(issues, Issue{
			Level:      LevelError,
			Type:       IssueMissingReference,
			FilePath:   filePath,
			Line:       ref.Line,
			Message:    fmt.Sprintf("Reference [[%s]] not found", ref.TargetRaw),
			Value:      ref.TargetRaw,
			FixCommand: fixCmd,
			FixHint:    fixHint,
		})

		// Track this missing reference with type inference
		v.trackMissingRef(ref.TargetRaw, filePath, sourceObjectID, ref.Line, targetType, fieldName)
	} else {
		// Reference resolved successfully - perform additional checks

		// Check if short ref could be a full path (for better clarity)
		if !strings.Contains(ref.TargetRaw, "/") && strings.Contains(result.TargetID, "/") {
			// Short ref that resolved to a full path - suggest using full path
			// Use displayID to strip directory prefix (e.g., "objects/") for cleaner suggestions
			suggestedID := v.displayID(result.TargetID)
			v.shortRefs[ref.TargetRaw] = suggestedID
			issues = append(issues, Issue{
				Level:    LevelWarning,
				Type:     IssueShortRefCouldBeFullPath,
				FilePath: filePath,
				Line:     ref.Line,
				Message:  fmt.Sprintf("Short reference [[%s]] could be written as [[%s]] for clarity", ref.TargetRaw, suggestedID),
				Value:    ref.TargetRaw,
				FixHint:  fmt.Sprintf("Consider using full path: [[%s]]", suggestedID),
			})
		}

		// Validate target type if specified (e.g., for ref fields with target constraint)
		if targetType != "" && len(v.objectTypes) > 0 {
			actualType, exists := v.objectTypes[result.TargetID]
			if exists && actualType != targetType {
				issues = append(issues, Issue{
					Level:    LevelError,
					Type:     IssueWrongTargetType,
					FilePath: filePath,
					Line:     ref.Line,
					Message:  fmt.Sprintf("Field '%s' expects type '%s', but [[%s]] is type '%s'", fieldName, targetType, ref.TargetRaw, actualType),
					Value:    ref.TargetRaw,
					FixHint:  fmt.Sprintf("Reference a '%s' object instead, or change the field's target type", targetType),
				})
			}
		}
	}

	return issues
}

func looksLikeAssetReference(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	base, _, _ := paths.ParseSectionID(target)
	if idx := strings.IndexAny(base, "?#"); idx >= 0 {
		base = base[:idx]
	}
	ext := strings.ToLower(filepath.Ext(base))
	return ext != "" && ext != ".md"
}

func formatAmbiguousRefMessage(raw string, result resolver.ResolveResult, displayID func(string) string) string {
	if len(result.Matches) == 0 {
		return fmt.Sprintf("Reference [[%s]] is ambiguous", raw)
	}

	matchDetails := make([]string, 0, len(result.Matches))
	for _, match := range result.Matches {
		source := result.MatchSources[match]
		label := labelForMatchSource(source)
		matchDetails = append(matchDetails, fmt.Sprintf("%s → %s", label, displayID(match)))
	}

	return fmt.Sprintf("Reference [[%s]] is ambiguous: %s", raw, strings.Join(matchDetails, "; "))
}

func formatAmbiguousRefFixHint(result resolver.ResolveResult, displayID func(string) string) string {
	if len(result.Matches) == 0 {
		return "Use a more specific path to disambiguate"
	}

	example := displayID(result.Matches[0])
	hint := fmt.Sprintf("Use a more specific path (e.g., [[%s]])", example)
	if hasSource(result.MatchSources, "alias") || hasSource(result.MatchSources, "short_name") || hasSource(result.MatchSources, "name_field") {
		hint += " or rename the conflicting alias/short name"
	}
	return hint
}

func labelForMatchSource(source string) string {
	switch source {
	case "alias":
		return "alias"
	case "name_field":
		return "name field"
	case "object_id":
		return "object id"
	case "short_name":
		return "short name"
	case "suffix_match":
		return "suffix match"
	case "date":
		return "date"
	default:
		return "match"
	}
}

func hasSource(matchSources map[string]string, source string) bool {
	for _, s := range matchSources {
		if s == source {
			return true
		}
	}
	return false
}

// trackMissingRef records a missing reference with type inference.
func (v *Validator) trackMissingRef(targetPath, sourceFile, sourceObjectID string, line int, targetType, fieldName string) {
	// Normalize path (remove .md extension if present, treat as file path)
	normalizedPath := targetPath

	// If we already have this ref with higher confidence, don't downgrade
	if existing, ok := v.missingRefs[normalizedPath]; ok {
		if existing.Confidence >= ConfidenceCertain {
			return // Already have certain confidence
		}
		if targetType != "" {
			// Upgrade to certain confidence
			existing.InferredType = targetType
			existing.Confidence = ConfidenceCertain
			existing.FieldSource = fieldName
			existing.SourceObjectID = sourceObjectID
			return
		}
	}

	missing := &MissingRef{
		TargetPath:     normalizedPath,
		SourceFile:     sourceFile,
		SourceObjectID: sourceObjectID,
		Line:           line,
	}

	// Determine confidence and type
	if targetType != "" {
		// From a typed field - certain
		missing.InferredType = targetType
		missing.Confidence = ConfidenceCertain
		missing.FieldSource = fieldName
	} else {
		// Try to infer from path
		inferredType, confidence := v.inferTypeFromPath(normalizedPath)
		missing.InferredType = inferredType
		missing.Confidence = confidence
	}

	v.missingRefs[normalizedPath] = missing
}
