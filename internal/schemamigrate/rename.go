// Package schemamigrate orchestrates vault-wide migrations that follow schema
// changes. Schema document transformations remain in schemasvc; this package
// stages and applies the corresponding config, template, Markdown, reference,
// and path updates.
package schemamigrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemachange"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// writeSchemaWithInvalidation writes schema.yaml and records invalidation for
// auto-reindex. It returns the operation ID and classification for later
// application via schemachange.ApplyInvalidation.
func writeSchemaWithInvalidation(rt *vaultruntime.Runtime, schemaBytes []byte) (string, schemachange.Classification, error) {
	vaultPath := rt.VaultPath

	// Load before state
	beforeSchema, _ := schema.Load(vaultPath)

	// Parse after state from staged bytes
	afterResult, parseErr := schema.Parse(schemaBytes, paths.SchemaPath(vaultPath))
	if parseErr != nil {
		return "", schemachange.Classification{}, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to parse staged schema", parseErr)
	}
	afterSchema := afterResult.Schema

	// Record invalidation before writing
	operationID, classification, err := schemachange.RecordInvalidation(vaultPath, beforeSchema, afterSchema)
	if err != nil {
		return "", schemachange.Classification{}, svcerr.Wrap(codes.ErrInternal, "failed to record schema invalidation", err)
	}

	// Write schema.yaml
	if err := schemadoc.Write(vaultPath, schemaBytes); err != nil {
		return "", schemachange.Classification{}, schemasvc.MapSchemaDocError(err, "", codes.ErrSchemaNotFound)
	}

	return operationID, classification, nil
}

type FieldRenameConflict struct {
	FilePath      string `json:"file_path"`
	ConflictType  string `json:"conflict_type"`
	Message       string `json:"message"`
	Line          int    `json:"line,omitempty"`
	OldFieldFound bool   `json:"old_field_found,omitempty"`
	NewFieldFound bool   `json:"new_field_found,omitempty"`
}

type RenameFieldRequest struct {
	VaultPath string
	TypeName  string
	OldField  string
	NewField  string
	Confirm   bool
}

type RenameFieldResult struct {
	Preview        bool
	TypeName       string
	OldField       string
	NewField       string
	TotalChanges   int
	Changes        []schemasvc.FieldRenameChange
	ChangesApplied int
	Hint           string
}

type RenameTypeRequest struct {
	VaultPath         string
	OldName           string
	NewName           string
	Description       string
	Confirm           bool
	RenameDefaultPath bool
}

type RenameTypeResult struct {
	Preview                    bool
	OldName                    string
	NewName                    string
	TotalChanges               int
	Changes                    []schemasvc.TypeRenameChange
	Hint                       string
	DefaultPathRenameAvailable bool
	DefaultPathRenamed         bool
	DefaultPathOld             string
	DefaultPathNew             string
	OptionalTotalChanges       int
	OptionalChanges            []schemasvc.TypeRenameChange
	FilesToMove                int
	ChangesApplied             int
	FilesMoved                 int
	ReferenceFilesUpdated      int
}

type typeDirectoryMove struct {
	SourceRelPath      string
	DestinationRelPath string
	SourceID           string
	DestinationID      string
}

type typeDefaultPathRenamePlan struct {
	OldDefaultPath string
	NewDefaultPath string
	Moves          []typeDirectoryMove
}

type fieldRenamePlan struct {
	Changes       []schemasvc.FieldRenameChange
	SchemaYAML    []byte
	TemplateFiles map[string][]byte
	RavenYAML     []byte
	MarkdownFiles map[string][]byte
	Conflicts     []FieldRenameConflict
}

// typeRenamePlan combines the schema-only plan with every staged vault
// mutation. Preview and apply consume this same plan so their behavior cannot
// drift.
type typeRenamePlan struct {
	SchemaPlan      *schemasvc.TypeRenamePlan
	MarkdownFiles   map[string][]byte
	DefaultPathPlan *typeDefaultPathRenamePlan
	ReferenceFiles  map[string][]byte
	Changes         []schemasvc.TypeRenameChange
	OptionalChanges []schemasvc.TypeRenameChange
}

var frontmatterTypeKeyLine = regexp.MustCompile(`^type\s*:`)

func RenameField(rt *vaultruntime.Runtime, req RenameFieldRequest) (*RenameFieldResult, error) {
	req.VaultPath = rt.VaultPath
	typeName := strings.TrimSpace(req.TypeName)
	oldField := strings.TrimSpace(req.OldField)
	newField := strings.TrimSpace(req.NewField)
	if typeName == "" || oldField == "" || newField == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type and field names cannot be empty").WithSuggestion("Usage: rvn schema rename field <type> <old_field> <new_field>")
	}
	if oldField == newField {
		return nil, svcerr.New(codes.ErrInvalidInput, "old and new field names are the same")
	}
	if schema.IsBuiltinType(typeName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot rename fields on built-in type '%s'", typeName))
	}

	schemaDoc, err := loadSchemaDocument(req.VaultPath)
	if err != nil {
		return nil, err
	}
	sch := schemaDoc.Schema()
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName))
	}
	if typeDef == nil || typeDef.Fields == nil {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName))
	}
	if _, ok := typeDef.Fields[oldField]; !ok {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName))
	}
	if _, ok := typeDef.Fields[newField]; ok {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName))
	}

	plan, err := buildFieldRenamePlan(req.VaultPath, schemaDoc, typeName, oldField, newField, rt.VaultCfg)
	if err != nil {
		return nil, err
	}
	if len(plan.Conflicts) > 0 {
		return nil, svcerr.New(codes.ErrDataIntegrityBlock, fmt.Sprintf("field rename blocked by %d conflicts", len(plan.Conflicts))).WithSuggestion("Resolve conflicts (remove one of the duplicate keys) and retry").WithDetails(map[string]interface{}{
			"type":       typeName,
			"old_field":  oldField,
			"new_field":  newField,
			"conflicts":  plan.Conflicts,
			"hint":       "Conflicts occur when both old and new field keys are present in the same object/declaration.",
			"next_steps": "Fix conflicts, then re-run the command (preview first).",
		})
	}

	if !req.Confirm {
		return &RenameFieldResult{
			Preview:      true,
			TypeName:     typeName,
			OldField:     oldField,
			NewField:     newField,
			TotalChanges: len(plan.Changes),
			Changes:      plan.Changes,
			Hint:         "Run with --confirm to apply changes",
		}, nil
	}

	appliedChanges, err := applyFieldRenamePlan(rt, plan)
	if err != nil {
		return nil, err
	}
	return &RenameFieldResult{
		Preview:        false,
		TypeName:       typeName,
		OldField:       oldField,
		NewField:       newField,
		ChangesApplied: appliedChanges,
		Hint:           "Run 'rvn reindex --full' to update the index",
	}, nil
}

func RenameType(rt *vaultruntime.Runtime, req RenameTypeRequest) (*RenameTypeResult, error) {
	req.VaultPath = rt.VaultPath
	oldName := strings.TrimSpace(req.OldName)
	newName := strings.TrimSpace(req.NewName)
	if oldName == "" || newName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type names cannot be empty")
	}
	if oldName == newName {
		return nil, svcerr.New(codes.ErrInvalidInput, "old and new names are the same")
	}
	if schema.IsBuiltinType(oldName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be renamed", oldName))
	}
	if schema.IsBuiltinType(newName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot rename to '%s' - it's a built-in type", newName))
	}

	schemaDoc, err := loadSchemaDocument(req.VaultPath)
	if err != nil {
		return nil, err
	}
	sch := schemaDoc.Schema()
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "failed to load raven.yaml").WithSuggestion("Fix raven.yaml and try again")
	}
	vaultCfg := rt.VaultCfg
	oldTypeDef, exists := sch.Types[oldName]
	if !exists {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", oldName))
	}
	if _, exists := sch.Types[newName]; exists {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("type '%s' already exists", newName)).WithSuggestion("Choose a different name")
	}

	plan, err := buildTypeRenamePlan(req.VaultPath, schemaDoc, req.Description, oldName, newName, oldTypeDef, vaultCfg)
	if err != nil {
		return nil, err
	}
	defaultPathPlan := plan.DefaultPathPlan

	result := &RenameTypeResult{
		OldName:      oldName,
		NewName:      newName,
		TotalChanges: len(plan.Changes),
		Changes:      plan.Changes,
		Hint:         "Run with --confirm to apply changes",
	}
	if defaultPathPlan != nil {
		result.DefaultPathRenameAvailable = true
		result.DefaultPathOld = defaultPathPlan.OldDefaultPath
		result.DefaultPathNew = defaultPathPlan.NewDefaultPath
		result.OptionalTotalChanges = len(plan.OptionalChanges)
		result.OptionalChanges = plan.OptionalChanges
		result.FilesToMove = len(defaultPathPlan.Moves)
		result.Hint = "Run with --confirm to apply changes. Add --rename-default-path to also rename the default directory and move matching files."
	}

	if !req.Confirm {
		result.Preview = true
		return result, nil
	}

	applyDefaultPathRename := defaultPathPlan != nil && req.RenameDefaultPath
	if applyDefaultPathRename {
		if err := validateTypeDirectoryMoves(req.VaultPath, defaultPathPlan.Moves); err != nil {
			return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("cannot rename default directory: %v", err)).WithSuggestion("Use --confirm without --rename-default-path, or resolve destination conflicts and try again")
		}
	}

	appliedChanges, movedFiles, referenceFilesUpdated, err := applyTypeRenamePlan(rt, plan, applyDefaultPathRename)
	if err != nil {
		return nil, err
	}

	return &RenameTypeResult{
		Preview:                    false,
		OldName:                    oldName,
		NewName:                    newName,
		ChangesApplied:             appliedChanges,
		Hint:                       hintForTypeApply(defaultPathPlan != nil, applyDefaultPathRename),
		DefaultPathRenameAvailable: defaultPathPlan != nil,
		DefaultPathRenamed:         applyDefaultPathRename,
		DefaultPathOld:             defaultPathValue(defaultPathPlan, true),
		DefaultPathNew:             defaultPathValue(defaultPathPlan, false),
		FilesMoved:                 movedFiles,
		ReferenceFilesUpdated:      referenceFilesUpdated,
	}, nil
}

func buildTypeRenamePlan(
	vaultPath string,
	schemaDoc *schemadoc.Document,
	description, oldName, newName string,
	oldTypeDef *schema.TypeDefinition,
	vaultCfg *config.VaultConfig,
) (*typeRenamePlan, error) {
	oldDefaultPath := ""
	if oldTypeDef != nil {
		oldDefaultPath = oldTypeDef.DefaultPath
	}
	schemaPlan, err := schemasvc.BuildTypeRenamePlan(schemasvc.TypeRenamePlanRequest{
		SchemaDoc:      schemaDoc,
		OldName:        oldName,
		NewName:        newName,
		Description:    description,
		OldDefaultPath: oldDefaultPath,
	})
	if err != nil {
		return nil, err
	}

	plan := &typeRenamePlan{
		SchemaPlan:      schemaPlan,
		MarkdownFiles:   make(map[string][]byte),
		ReferenceFiles:  make(map[string][]byte),
		Changes:         append([]schemasvc.TypeRenameChange(nil), schemaPlan.Changes...),
		OptionalChanges: append([]schemasvc.TypeRenameChange(nil), schemaPlan.OptionalChanges...),
	}
	if schemaPlan.DefaultPathOld != "" {
		plan.DefaultPathPlan = &typeDefaultPathRenamePlan{
			OldDefaultPath: schemaPlan.DefaultPathOld,
			NewDefaultPath: schemaPlan.DefaultPathNew,
		}
	}

	movesBySource := make(map[string]typeDirectoryMove)
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		hasFileLevelOldType := false
		for _, obj := range result.Document.Objects {
			if obj.Type == oldName && !strings.Contains(obj.ID, "#") {
				hasFileLevelOldType = true
				break
			}
		}
		if !hasFileLevelOldType {
			return nil
		}

		if staged, ok := stageFrontmatterTypeRewrite(result.Document.RawContent, newName); ok {
			plan.MarkdownFiles[result.Path] = staged
			plan.Changes = append(plan.Changes, schemasvc.TypeRenameChange{
				FilePath:    result.RelativePath,
				ChangeType:  "frontmatter",
				Description: fmt.Sprintf("change type: %s → type: %s", oldName, newName),
				Line:        1,
			})
		}

		if plan.DefaultPathPlan != nil {
			if move, ok := planTypeDirectoryMove(result.RelativePath, newName, plan.DefaultPathPlan, vaultCfg); ok {
				if _, exists := movesBySource[move.SourceRelPath]; !exists {
					movesBySource[move.SourceRelPath] = move
					plan.DefaultPathPlan.Moves = append(plan.DefaultPathPlan.Moves, move)
					plan.OptionalChanges = append(plan.OptionalChanges, schemasvc.TypeRenameChange{
						FilePath:    move.SourceRelPath,
						ChangeType:  "directory_move",
						Description: fmt.Sprintf("move file '%s' → '%s'", move.SourceRelPath, move.DestinationRelPath),
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}

	if plan.DefaultPathPlan != nil && len(plan.DefaultPathPlan.Moves) > 0 {
		if err := plan.stageReferenceUpdates(vaultPath, vaultCfg); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func buildFieldRenamePlan(
	vaultPath string,
	schemaDoc *schemadoc.Document,
	typeName, oldField, newField string,
	vaultCfg *config.VaultConfig,
) (*fieldRenamePlan, error) {
	tokenOld := "{{field." + oldField + "}}"
	tokenNew := "{{field." + newField + "}}"

	schemaPlan, err := schemasvc.BuildFieldRenamePlan(schemasvc.FieldRenamePlanRequest{
		SchemaDoc: schemaDoc,
		TypeName:  typeName,
		OldField:  oldField,
		NewField:  newField,
	})
	if err != nil {
		return nil, err
	}

	plan := &fieldRenamePlan{
		SchemaYAML:    schemaPlan.SchemaYAML,
		TemplateFiles: make(map[string][]byte),
		MarkdownFiles: make(map[string][]byte),
		Changes:       append([]schemasvc.FieldRenameChange(nil), schemaPlan.Changes...),
		Conflicts:     make([]FieldRenameConflict, 0),
	}

	if schemaPlan.TemplateSpec != "" && looksLikeTemplatePath(schemaPlan.TemplateSpec) {
		absTemplate := filepath.Join(vaultPath, schemaPlan.TemplateSpec)
		if err := paths.ValidateWithinVault(vaultPath, absTemplate); err != nil {
			if !errors.Is(err, paths.ErrPathOutsideVault) {
				return nil, svcerr.Wrap(codes.ErrFileOutsideVault, err.Error(), err)
			}
		} else {
			templateContent, err := os.ReadFile(absTemplate)
			if err == nil {
				newContent := strings.ReplaceAll(string(templateContent), tokenOld, tokenNew)
				if newContent != string(templateContent) {
					plan.TemplateFiles[absTemplate] = []byte(newContent)
					rel, _ := paths.RelFromVault(vaultPath, absTemplate)
					plan.Changes = append(plan.Changes, schemasvc.FieldRenameChange{
						FilePath:    rel,
						ChangeType:  "template_file",
						Description: fmt.Sprintf("update template variable %s → %s", tokenOld, tokenNew),
					})
				}
			}
		}
	}

	changedQueries := false
	fieldRefPattern := regexp.MustCompile(`\.` + regexp.QuoteMeta(oldField) + `\b`)
	if vaultCfg != nil && vaultCfg.Queries != nil {
		for _, name := range sortedStringKeys(vaultCfg.Queries) {
			savedQuery := vaultCfg.Queries[name]
			if savedQuery == nil || savedQuery.Query == "" {
				continue
			}
			parsed, err := query.Parse(savedQuery.Query)
			if err != nil || parsed == nil {
				continue
			}
			if parsed.Type != query.QueryTypeObject || parsed.TypeName != typeName {
				continue
			}
			newQuery := fieldRefPattern.ReplaceAllString(savedQuery.Query, "."+newField)
			if newQuery != savedQuery.Query {
				savedQuery.Query = newQuery
				changedQueries = true
				plan.Changes = append(plan.Changes, schemasvc.FieldRenameChange{
					FilePath:    "raven.yaml",
					ChangeType:  "saved_query",
					Description: fmt.Sprintf("update saved query '%s': .%s → .%s", name, oldField, newField),
				})
			}
		}
	}
	if changedQueries {
		configOut, err := yaml.Marshal(vaultCfg)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
		}
		plan.RavenYAML = configOut
	}

	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		original := result.Document.RawContent
		lines := strings.Split(original, "\n")
		startLine, endLine, frontmatterOK := parser.FrontmatterBounds(lines)
		if !frontmatterOK || endLine == -1 {
			return nil
		}

		frontmatterContent := strings.Join(lines[startLine+1:endLine], "\n")
		frontmatter, ok := decodeYAMLMap([]byte(frontmatterContent))
		if !ok {
			return nil
		}
		if objectType, ok := frontmatter["type"].(string); !ok || objectType != typeName {
			return nil
		}

		_, oldPresent := frontmatter[oldField]
		_, newPresent := frontmatter[newField]
		if oldPresent && newPresent {
			plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
				FilePath:      result.RelativePath,
				ConflictType:  "frontmatter",
				Message:       fmt.Sprintf("frontmatter contains both '%s' and '%s'", oldField, newField),
				Line:          1,
				OldFieldFound: true,
				NewFieldFound: true,
			})
			return nil
		}
		if !oldPresent {
			return nil
		}

		frontmatter[newField] = frontmatter[oldField]
		delete(frontmatter, oldField)
		newFrontmatter, ok := marshalYAMLMap(frontmatter)
		if !ok {
			return nil
		}

		var output strings.Builder
		output.WriteString("---\n")
		output.Write(newFrontmatter)
		output.WriteString("---")
		if endLine+1 < len(lines) {
			output.WriteString("\n")
			output.WriteString(strings.Join(lines[endLine+1:], "\n"))
		}

		plan.MarkdownFiles[result.Path] = []byte(output.String())
		plan.Changes = append(plan.Changes, schemasvc.FieldRenameChange{
			FilePath:    result.RelativePath,
			ChangeType:  "frontmatter",
			Description: fmt.Sprintf("rename frontmatter key '%s:' → '%s:' for type '%s'", oldField, newField, typeName),
			Line:        1,
		})
		return nil
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}

	return plan, nil
}

func applyFieldRenamePlan(rt *vaultruntime.Runtime, plan *fieldRenamePlan) (int, error) {
	vaultPath := rt.VaultPath
	appliedChanges := 0
	var operationID string
	var classification schemachange.Classification

	if len(plan.SchemaYAML) > 0 {
		opID, classif, err := writeSchemaWithInvalidation(rt, plan.SchemaYAML)
		if err != nil {
			return 0, err
		}
		operationID = opID
		classification = classif
		appliedChanges++
	}
	for _, path := range sortedStringKeys(plan.TemplateFiles) {
		if err := atomicfile.WriteFile(path, plan.TemplateFiles[path], 0o644); err != nil {
			return 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
		}
		appliedChanges++
	}
	if len(plan.RavenYAML) > 0 {
		configPath := filepath.Join(vaultPath, "raven.yaml")
		if err := atomicfile.WriteFile(configPath, plan.RavenYAML, 0o644); err != nil {
			return 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
		}
		appliedChanges++
	}
	for _, path := range sortedStringKeys(plan.MarkdownFiles) {
		if err := atomicfile.WriteFile(path, plan.MarkdownFiles[path], 0o644); err != nil {
			return 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
		}
		appliedChanges++
	}

	// Apply invalidation (runs auto-reindex if enabled)
	if operationID != "" {
		if err := rt.ReloadSchema(true); err != nil {
			return 0, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to reload schema after rename", err)
		}
		// Attempt to apply invalidation. If it fails, the schema write still succeeded
		// and the journal entry persists, so a manual reindex will recover.
		_ = schemachange.ApplyInvalidation(rt, operationID, classification)
	}

	return appliedChanges, nil
}

func applyTypeRenamePlan(rt *vaultruntime.Runtime, plan *typeRenamePlan, applyDefaultPathRename bool) (int, int, int, error) {
	vaultPath := rt.VaultPath
	appliedChanges := plan.SchemaPlan.CoreSchemaMutations
	schemaBytes := plan.SchemaPlan.SchemaYAML
	if applyDefaultPathRename && plan.SchemaPlan.SchemaYAMLWithDefaultPath != nil {
		schemaBytes = plan.SchemaPlan.SchemaYAMLWithDefaultPath
		if plan.SchemaPlan.DefaultPathMutation {
			appliedChanges++
		}
	}

	// Write schema with invalidation tracking
	operationID, classification, err := writeSchemaWithInvalidation(rt, schemaBytes)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, path := range sortedStringKeys(plan.MarkdownFiles) {
		if err := atomicfile.WriteFile(path, plan.MarkdownFiles[path], 0o644); err != nil {
			return 0, 0, 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
		}
		appliedChanges++
	}

	movedFiles := 0
	referenceFilesUpdated := 0
	if applyDefaultPathRename {
		moved, err := applyTypeDirectoryMoves(vaultPath, plan.DefaultPathPlan.Moves)
		if err != nil {
			return 0, 0, 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err).WithSuggestion("Some files may have been renamed; review the vault and run 'rvn reindex --full'")
		}
		movedFiles = moved
		for _, path := range sortedStringKeys(plan.ReferenceFiles) {
			if err := atomicfile.WriteFile(path, plan.ReferenceFiles[path], 0o644); err != nil {
				return 0, 0, 0, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err).WithSuggestion("Some files may have been renamed; review the vault and run 'rvn reindex --full'")
			}
			referenceFilesUpdated++
		}
		appliedChanges += movedFiles + referenceFilesUpdated
	}

	// Apply invalidation (runs auto-reindex if enabled)
	if operationID != "" {
		if err := rt.ReloadSchema(true); err != nil {
			return 0, 0, 0, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to reload schema after rename", err)
		}
		// Attempt to apply invalidation. If it fails, the schema write still succeeded
		// and the journal entry persists, so a manual reindex will recover.
		_ = schemachange.ApplyInvalidation(rt, operationID, classification)
	}

	return appliedChanges, movedFiles, referenceFilesUpdated, nil
}

func stageFrontmatterTypeRewrite(raw, newName string) ([]byte, bool) {
	lines := strings.Split(raw, "\n")
	start, end, ok := parser.FrontmatterBounds(lines)
	if !ok || end == -1 {
		return nil, false
	}
	for i := start + 1; i < end; i++ {
		if !frontmatterTypeKeyLine.MatchString(lines[i]) {
			continue
		}
		colon := strings.IndexByte(lines[i], ':')
		if colon < 0 {
			continue
		}
		newLine := "type: " + newName
		if comment := trailingYAMLComment(lines[i][colon+1:]); comment != "" {
			newLine += " " + comment
		}
		lines[i] = newLine
		return []byte(strings.Join(lines, "\n")), true
	}
	return nil, false
}

func trailingYAMLComment(valueSegment string) string {
	if hash := strings.IndexByte(valueSegment, '#'); hash >= 0 {
		return strings.TrimSpace(valueSegment[hash:])
	}
	return ""
}

func (plan *typeRenamePlan) stageReferenceUpdates(vaultPath string, vaultCfg *config.VaultConfig) error {
	idMoves := make(map[string]string, len(plan.DefaultPathPlan.Moves))
	destBySourceRel := make(map[string]string, len(plan.DefaultPathPlan.Moves))
	for _, move := range plan.DefaultPathPlan.Moves {
		idMoves[move.SourceID] = move.DestinationID
		destBySourceRel[move.SourceRelPath] = filepath.Join(vaultPath, move.DestinationRelPath)
	}

	oldIDs := make([]string, 0, len(idMoves))
	for oldID := range idMoves {
		oldIDs = append(oldIDs, oldID)
	}
	sort.SliceStable(oldIDs, func(i, j int) bool {
		return len(oldIDs[i]) > len(oldIDs[j])
	})

	objectRoot := ""
	pageRoot := ""
	if vaultCfg != nil {
		objectRoot = vaultCfg.GetObjectsRoot()
		pageRoot = vaultCfg.GetPagesRoot()
	}

	return vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		base := result.Document.RawContent
		if staged, ok := plan.MarkdownFiles[result.Path]; ok {
			base = string(staged)
		}
		updated := base
		for _, oldID := range oldIDs {
			updated = objectsvc.ReplaceAllRefVariants(updated, oldID, oldID, idMoves[oldID], objectRoot, pageRoot)
		}
		if updated == base {
			return nil
		}

		finalPath := result.Path
		if destination, ok := destBySourceRel[result.RelativePath]; ok {
			finalPath = destination
		}
		plan.ReferenceFiles[finalPath] = []byte(updated)
		plan.OptionalChanges = append(plan.OptionalChanges, schemasvc.TypeRenameChange{
			FilePath:    result.RelativePath,
			ChangeType:  "reference_update",
			Description: fmt.Sprintf("update references after directory move in '%s'", result.RelativePath),
		})
		return nil
	})
}

func looksLikeTemplatePath(value string) bool {
	if value == "" {
		return false
	}
	if strings.Contains(value, "/") || strings.HasSuffix(value, ".md") || strings.HasPrefix(value, "templates") {
		return true
	}
	if strings.Contains(value, "\n") {
		return false
	}
	matched, _ := regexp.MatchString(`^[\w.-]+$`, value)
	return matched && len(value) < 100
}

func planTypeDirectoryMove(
	relPath, newName string,
	plan *typeDefaultPathRenamePlan,
	vaultCfg *config.VaultConfig,
) (typeDirectoryMove, bool) {
	if plan == nil || vaultCfg == nil {
		return typeDirectoryMove{}, false
	}
	sourceRel := paths.NormalizeVaultRelPath(relPath)
	sourceID := vaultCfg.FilePathToObjectID(sourceRel)
	if !strings.HasPrefix(sourceID, plan.OldDefaultPath) {
		return typeDirectoryMove{}, false
	}
	suffix := strings.TrimPrefix(sourceID, plan.OldDefaultPath)
	if suffix == "" {
		return typeDirectoryMove{}, false
	}
	destinationID := plan.NewDefaultPath + suffix
	destinationRel := filepath.ToSlash(vaultCfg.ObjectIDToFilePath(destinationID, newName))
	if sourceRel == destinationRel {
		return typeDirectoryMove{}, false
	}
	return typeDirectoryMove{
		SourceRelPath:      sourceRel,
		DestinationRelPath: destinationRel,
		SourceID:           sourceID,
		DestinationID:      destinationID,
	}, true
}

func validateTypeDirectoryMoves(vaultPath string, moves []typeDirectoryMove) error {
	if len(moves) == 0 {
		return nil
	}

	destinations := make(map[string]string, len(moves))
	sources := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destinationAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		sources[filepath.Clean(sourceAbs)] = struct{}{}

		if _, err := os.Stat(sourceAbs); err != nil {
			return fmt.Errorf("source file does not exist: %s", move.SourceRelPath)
		}
		if existingSource, exists := destinations[filepath.Clean(destinationAbs)]; exists && existingSource != move.SourceRelPath {
			return fmt.Errorf("multiple files would move to '%s'", move.DestinationRelPath)
		}
		destinations[filepath.Clean(destinationAbs)] = move.SourceRelPath
	}

	for _, move := range moves {
		destinationAbs := filepath.Clean(filepath.Join(vaultPath, move.DestinationRelPath))
		if _, isSource := sources[destinationAbs]; isSource {
			continue
		}
		if _, err := os.Stat(destinationAbs); err == nil {
			return fmt.Errorf("destination already exists: %s", move.DestinationRelPath)
		}
	}
	return nil
}

func applyTypeDirectoryMoves(vaultPath string, moves []typeDirectoryMove) (int, error) {
	orderedMoves := append([]typeDirectoryMove(nil), moves...)
	sort.SliceStable(orderedMoves, func(i, j int) bool {
		return len(orderedMoves[i].SourceRelPath) > len(orderedMoves[j].SourceRelPath)
	})
	for _, move := range orderedMoves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destinationAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		if err := os.MkdirAll(filepath.Dir(destinationAbs), 0o755); err != nil {
			return 0, err
		}
		if err := os.Rename(sourceAbs, destinationAbs); err != nil {
			return 0, err
		}
	}
	return len(orderedMoves), nil
}

func hintForTypeApply(hasDefaultPathPlan, applied bool) string {
	if hasDefaultPathPlan && !applied {
		return "Run 'rvn reindex --full' to update the index. Use --rename-default-path to also rename the default directory."
	}
	return "Run 'rvn reindex --full' to update the index"
}

func defaultPathValue(plan *typeDefaultPathRenamePlan, old bool) string {
	if plan == nil {
		return ""
	}
	if old {
		return plan.OldDefaultPath
	}
	return plan.NewDefaultPath
}

func loadSchemaDocument(vaultPath string) (*schemadoc.Document, error) {
	doc, err := schemadoc.Load(vaultPath)
	if err != nil {
		return nil, schemasvc.MapSchemaDocError(err, "Run 'rvn init' first", codes.ErrSchemaNotFound)
	}
	return doc, nil
}

func sortedStringKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func decodeYAMLMap(data []byte) (map[string]interface{}, bool) {
	var values map[string]interface{}
	if yaml.Unmarshal(data, &values) != nil {
		return nil, false
	}
	if values == nil {
		values = make(map[string]interface{})
	}
	return values, true
}

func marshalYAMLMap(values map[string]interface{}) ([]byte, bool) {
	data, err := yaml.Marshal(values)
	return data, err == nil
}
