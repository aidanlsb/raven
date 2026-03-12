package schemasvc

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
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

type TypeRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

type FieldRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
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
	Changes        []FieldRenameChange
	ChangesApplied int
	Hint           string
}

type RenameTypeRequest struct {
	VaultPath         string
	OldName           string
	NewName           string
	Confirm           bool
	RenameDefaultPath bool
}

type RenameTypeResult struct {
	Preview                    bool
	OldName                    string
	NewName                    string
	TotalChanges               int
	Changes                    []TypeRenameChange
	Hint                       string
	DefaultPathRenameAvailable bool
	DefaultPathRenamed         bool
	DefaultPathOld             string
	DefaultPathNew             string
	OptionalTotalChanges       int
	OptionalChanges            []TypeRenameChange
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
	Changes       []FieldRenameChange
	SchemaYAML    []byte
	TemplateFiles map[string][]byte
	RavenYAML     []byte
	MarkdownFiles map[string][]byte
	Conflicts     []FieldRenameConflict
}

func RenameField(req RenameFieldRequest) (*RenameFieldResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	oldField := strings.TrimSpace(req.OldField)
	newField := strings.TrimSpace(req.NewField)
	if typeName == "" || oldField == "" || newField == "" {
		return nil, newError(ErrorInvalidInput, "type and field names cannot be empty", "Usage: rvn schema rename field <type> <old_field> <new_field>", nil, nil)
	}
	if oldField == newField {
		return nil, newError(ErrorInvalidInput, "old and new field names are the same", "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("cannot rename fields on built-in type '%s'", typeName), "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}
	if typeDef == nil || typeDef.Fields == nil {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName), "", nil, nil)
	}
	if _, ok := typeDef.Fields[oldField]; !ok {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName), "", nil, nil)
	}
	if _, ok := typeDef.Fields[newField]; ok {
		return nil, newError(ErrorObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName), "", nil, nil)
	}

	plan, err := buildFieldRenamePlan(req.VaultPath, typeName, oldField, newField)
	if err != nil {
		return nil, err
	}
	if len(plan.Conflicts) > 0 {
		return nil, newError(
			ErrorDataIntegrity,
			fmt.Sprintf("field rename blocked by %d conflicts", len(plan.Conflicts)),
			"Resolve conflicts (remove one of the duplicate keys) and retry",
			map[string]interface{}{
				"type":       typeName,
				"old_field":  oldField,
				"new_field":  newField,
				"conflicts":  plan.Conflicts,
				"hint":       "Conflicts occur when both old and new field keys are present in the same object/declaration.",
				"next_steps": "Fix conflicts, then re-run the command (preview first).",
			},
			nil,
		)
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

	appliedChanges := 0
	if len(plan.SchemaYAML) > 0 {
		schemaPath := paths.SchemaPath(req.VaultPath)
		if err := atomicfile.WriteFile(schemaPath, plan.SchemaYAML, 0o644); err != nil {
			return nil, newError(ErrorFileWrite, err.Error(), "", nil, err)
		}
		appliedChanges++
	}

	if len(plan.TemplateFiles) > 0 {
		pathsSorted := make([]string, 0, len(plan.TemplateFiles))
		for p := range plan.TemplateFiles {
			pathsSorted = append(pathsSorted, p)
		}
		sort.Strings(pathsSorted)
		for _, p := range pathsSorted {
			if err := atomicfile.WriteFile(p, plan.TemplateFiles[p], 0o644); err != nil {
				return nil, newError(ErrorFileWrite, err.Error(), "", nil, err)
			}
			appliedChanges++
		}
	}

	if len(plan.RavenYAML) > 0 {
		cfgPath := filepath.Join(req.VaultPath, "raven.yaml")
		if err := atomicfile.WriteFile(cfgPath, plan.RavenYAML, 0o644); err != nil {
			return nil, newError(ErrorFileWrite, err.Error(), "", nil, err)
		}
		appliedChanges++
	}

	if len(plan.MarkdownFiles) > 0 {
		pathsSorted := make([]string, 0, len(plan.MarkdownFiles))
		for p := range plan.MarkdownFiles {
			pathsSorted = append(pathsSorted, p)
		}
		sort.Strings(pathsSorted)
		for _, p := range pathsSorted {
			if err := atomicfile.WriteFile(p, plan.MarkdownFiles[p], 0o644); err != nil {
				return nil, newError(ErrorFileWrite, err.Error(), "", nil, err)
			}
			appliedChanges++
		}
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

func RenameType(req RenameTypeRequest) (*RenameTypeResult, error) {
	oldName := strings.TrimSpace(req.OldName)
	newName := strings.TrimSpace(req.NewName)
	if oldName == "" || newName == "" {
		return nil, newError(ErrorInvalidInput, "type names cannot be empty", "", nil, nil)
	}
	if oldName == newName {
		return nil, newError(ErrorInvalidInput, "old and new names are the same", "", nil, nil)
	}
	if schema.IsBuiltinType(oldName) {
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be renamed", oldName), "", nil, nil)
	}
	if schema.IsBuiltinType(newName) {
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("cannot rename to '%s' - it's a built-in type", newName), "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorConfigInvalid, "failed to load raven.yaml", "Fix raven.yaml and try again", nil, err)
	}

	if _, exists := sch.Types[oldName]; !exists {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", oldName), "", nil, nil)
	}
	if _, exists := sch.Types[newName]; exists {
		return nil, newError(ErrorObjectExists, fmt.Sprintf("type '%s' already exists", newName), "Choose a different name", nil, nil)
	}

	changes := make([]TypeRenameChange, 0)
	optionalChanges := make([]TypeRenameChange, 0)

	changes = append(changes, TypeRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_type",
		Description: fmt.Sprintf("rename type '%s' to '%s'", oldName, newName),
	})

	for typeName, typeDef := range sch.Types {
		if typeDef == nil || typeDef.Fields == nil {
			continue
		}
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			if fieldDef.Target == oldName {
				changes = append(changes, TypeRenameChange{
					FilePath:    "schema.yaml",
					ChangeType:  "schema_ref_target",
					Description: fmt.Sprintf("update field '%s.%s' target from '%s' to '%s'", typeName, fieldName, oldName, newName),
				})
			}
		}
	}

	var defaultPathPlan *typeDefaultPathRenamePlan
	if oldTypeDef := sch.Types[oldName]; oldTypeDef != nil {
		if suggestedPath, ok := suggestRenamedDefaultPath(oldTypeDef.DefaultPath, oldName, newName); ok {
			defaultPathPlan = &typeDefaultPathRenamePlan{
				OldDefaultPath: paths.NormalizeDirRoot(oldTypeDef.DefaultPath),
				NewDefaultPath: suggestedPath,
			}
			optionalChanges = append(optionalChanges, TypeRenameChange{
				FilePath:    "schema.yaml",
				ChangeType:  "schema_default_path",
				Description: fmt.Sprintf("update default_path '%s' → '%s' for type '%s'", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath, newName),
			})
		}
	}

	movesBySource := make(map[string]typeDirectoryMove)
	err = vault.WalkMarkdownFiles(req.VaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}

		content, readErr := os.ReadFile(result.Path)
		if readErr != nil {
			return readErr
		}
		lines := strings.Split(string(content), "\n")

		hasFileLevelOldType := false
		for _, obj := range result.Document.Objects {
			if obj.ObjectType == oldName && !strings.Contains(obj.ID, "#") {
				hasFileLevelOldType = true
				changes = append(changes, TypeRenameChange{
					FilePath:    result.RelativePath,
					ChangeType:  "frontmatter",
					Description: fmt.Sprintf("change type: %s → type: %s", oldName, newName),
					Line:        1,
				})
			}
		}

		if hasFileLevelOldType && defaultPathPlan != nil {
			if move, ok := planTypeDirectoryMove(result.RelativePath, newName, defaultPathPlan, vaultCfg); ok {
				if _, exists := movesBySource[move.SourceRelPath]; !exists {
					movesBySource[move.SourceRelPath] = move
					defaultPathPlan.Moves = append(defaultPathPlan.Moves, move)
					optionalChanges = append(optionalChanges, TypeRenameChange{
						FilePath:    move.SourceRelPath,
						ChangeType:  "directory_move",
						Description: fmt.Sprintf("move file '%s' → '%s'", move.SourceRelPath, move.DestinationRelPath),
					})
				}
			}
		}

		embeddedPattern := fmt.Sprintf("::%s(", oldName)
		for lineNum, line := range lines {
			if strings.Contains(line, embeddedPattern) {
				changes = append(changes, TypeRenameChange{
					FilePath:    result.RelativePath,
					ChangeType:  "embedded",
					Description: fmt.Sprintf("change ::%s(...) → ::%s(...)", oldName, newName),
					Line:        lineNum + 1,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, newError(ErrorInternal, err.Error(), "", nil, err)
	}

	result := &RenameTypeResult{
		OldName:      oldName,
		NewName:      newName,
		TotalChanges: len(changes),
		Changes:      changes,
		Hint:         "Run with --confirm to apply changes",
	}
	if defaultPathPlan != nil {
		result.DefaultPathRenameAvailable = true
		result.DefaultPathOld = defaultPathPlan.OldDefaultPath
		result.DefaultPathNew = defaultPathPlan.NewDefaultPath
		result.OptionalTotalChanges = len(optionalChanges)
		result.OptionalChanges = optionalChanges
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
			return nil, newError(
				ErrorValidation,
				fmt.Sprintf("cannot rename default directory: %v", err),
				"Use --confirm without --rename-default-path, or resolve destination conflicts and try again",
				nil,
				nil,
			)
		}
	}

	appliedChanges := 0

	schemaData, err := os.ReadFile(paths.SchemaPath(req.VaultPath))
	if err != nil {
		return nil, newError(ErrorFileRead, err.Error(), "", nil, err)
	}
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(schemaData, &schemaDoc); err != nil {
		return nil, newError(ErrorSchemaInvalid, err.Error(), "", nil, err)
	}
	typesNode, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
	}

	if typeDef, exists := typesNode[oldName]; exists {
		typesNode[newName] = typeDef
		delete(typesNode, oldName)
		appliedChanges++
	}

	if applyDefaultPathRename {
		if typeDefAny, exists := typesNode[newName]; exists {
			if typeDefMap, ok := typeDefAny.(map[string]interface{}); ok {
				typeDefMap["default_path"] = defaultPathPlan.NewDefaultPath
				appliedChanges++
			}
		}
	}

	for _, typeDef := range typesNode {
		typeMap, ok := typeDef.(map[string]interface{})
		if !ok {
			continue
		}
		fields, ok := typeMap["fields"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, fieldDef := range fields {
			fieldMap, ok := fieldDef.(map[string]interface{})
			if !ok {
				continue
			}
			if target, ok := fieldMap["target"].(string); ok && target == oldName {
				fieldMap["target"] = newName
				appliedChanges++
			}
		}
	}

	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return nil, newError(ErrorInternal, err.Error(), "", nil, err)
	}
	if err := atomicfile.WriteFile(paths.SchemaPath(req.VaultPath), output, 0o644); err != nil {
		return nil, newError(ErrorFileWrite, err.Error(), "", nil, err)
	}

	err = vault.WalkMarkdownFiles(req.VaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		content, readErr := os.ReadFile(result.Path)
		if readErr != nil {
			return readErr
		}

		originalContent := string(content)
		newContent := originalContent
		modified := false

		frontmatterPattern := regexp.MustCompile(`(?m)^type:\s*` + regexp.QuoteMeta(oldName) + `\s*$`)
		if frontmatterPattern.MatchString(newContent) {
			newContent = frontmatterPattern.ReplaceAllString(newContent, "type: "+newName)
			modified = true
			appliedChanges++
		}

		embeddedPattern := regexp.MustCompile(`::` + regexp.QuoteMeta(oldName) + `\(`)
		if embeddedPattern.MatchString(newContent) {
			newContent = embeddedPattern.ReplaceAllString(newContent, "::"+newName+"(")
			modified = true
			appliedChanges++
		}

		if modified {
			if err := atomicfile.WriteFile(result.Path, []byte(newContent), 0o644); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, newError(ErrorInternal, err.Error(), "", nil, err)
	}

	movedFiles := 0
	referenceFilesUpdated := 0
	if applyDefaultPathRename {
		movedFiles, referenceFilesUpdated, err = applyTypeDirectoryRename(req.VaultPath, vaultCfg, defaultPathPlan.Moves)
		if err != nil {
			return nil, newError(ErrorFileWrite, err.Error(), "Some files may have been renamed; review the vault and run 'rvn reindex --full'", nil, err)
		}
		appliedChanges += movedFiles + referenceFilesUpdated
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

func buildFieldRenamePlan(vaultPath, typeName, oldField, newField string) (*fieldRenamePlan, error) {
	tokenOld := "{{field." + oldField + "}}"
	tokenNew := "{{field." + newField + "}}"

	plan := &fieldRenamePlan{
		TemplateFiles: make(map[string][]byte),
		MarkdownFiles: make(map[string][]byte),
		Changes:       []FieldRenameChange{},
		Conflicts:     []FieldRenameConflict{},
	}

	schemaData, err := os.ReadFile(paths.SchemaPath(vaultPath))
	if err != nil {
		return nil, newError(ErrorFileRead, err.Error(), "", nil, err)
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(schemaData, &schemaDoc); err != nil {
		return nil, newError(ErrorSchemaInvalid, err.Error(), "", nil, err)
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
	}
	typeNodeAny, ok := types[typeName]
	if !ok {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}
	typeNode, ok := typeNodeAny.(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, fmt.Sprintf("type '%s' has invalid definition", typeName), "", nil, nil)
	}
	fieldsAny, ok := typeNode["fields"]
	if !ok {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName), "", nil, nil)
	}
	fields, ok := fieldsAny.(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, fmt.Sprintf("type '%s' fields are invalid", typeName), "", nil, nil)
	}

	_, hasOld := fields[oldField]
	_, hasNew := fields[newField]
	if hasOld && hasNew {
		return nil, newError(
			ErrorObjectExists,
			fmt.Sprintf("type '%s' already has both '%s' and '%s' fields", typeName, oldField, newField),
			"Choose a different new field name or remove one field first",
			nil,
			nil,
		)
	}
	if hasNew {
		return nil, newError(
			ErrorObjectExists,
			fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName),
			"Choose a different new field name",
			nil,
			nil,
		)
	}
	if !hasOld {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName), "", nil, nil)
	}

	fields[newField] = fields[oldField]
	delete(fields, oldField)
	plan.Changes = append(plan.Changes, FieldRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_field",
		Description: fmt.Sprintf("rename field '%s' → '%s' on type '%s'", oldField, newField, typeName),
	})

	if nf, ok := typeNode["name_field"].(string); ok && nf == oldField {
		typeNode["name_field"] = newField
		plan.Changes = append(plan.Changes, FieldRenameChange{
			FilePath:    "schema.yaml",
			ChangeType:  "schema_name_field",
			Description: fmt.Sprintf("update name_field: %s → %s", oldField, newField),
		})
	}

	if tmplSpec, ok := typeNode["template"].(string); ok && tmplSpec != "" {
		if looksLikeTemplatePath(tmplSpec) {
			absTmpl := filepath.Join(vaultPath, tmplSpec)
			if err := paths.ValidateWithinVault(vaultPath, absTmpl); err != nil {
				if !errors.Is(err, paths.ErrPathOutsideVault) {
					return nil, newError(ErrorFileOutside, err.Error(), "", nil, err)
				}
			} else {
				tmplContent, err := os.ReadFile(absTmpl)
				if err == nil {
					newContent := strings.ReplaceAll(string(tmplContent), tokenOld, tokenNew)
					if newContent != string(tmplContent) {
						plan.TemplateFiles[absTmpl] = []byte(newContent)
						rel, _ := filepath.Rel(vaultPath, absTmpl)
						plan.Changes = append(plan.Changes, FieldRenameChange{
							FilePath:    rel,
							ChangeType:  "template_file",
							Description: fmt.Sprintf("update template variable %s → %s", tokenOld, tokenNew),
						})
					}
				}
			}
		}
	}

	schemaOut, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return nil, newError(ErrorInternal, err.Error(), "", nil, err)
	}
	plan.SchemaYAML = schemaOut

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, newError(ErrorFileRead, err.Error(), "", nil, err)
	}

	changedQueries := false
	fieldRefPattern := regexp.MustCompile(`\.` + regexp.QuoteMeta(oldField) + `\b`)

	if vaultCfg != nil && vaultCfg.Queries != nil {
		qNames := make([]string, 0, len(vaultCfg.Queries))
		for name := range vaultCfg.Queries {
			qNames = append(qNames, name)
		}
		sort.Strings(qNames)
		for _, name := range qNames {
			q := vaultCfg.Queries[name]
			if q == nil || q.Query == "" {
				continue
			}
			parsed, err := query.Parse(q.Query)
			if err != nil || parsed == nil {
				continue
			}
			if parsed.Type != query.QueryTypeObject || parsed.TypeName != typeName {
				continue
			}
			newQuery := fieldRefPattern.ReplaceAllString(q.Query, "."+newField)
			if newQuery != q.Query {
				q.Query = newQuery
				changedQueries = true
				plan.Changes = append(plan.Changes, FieldRenameChange{
					FilePath:    "raven.yaml",
					ChangeType:  "saved_query",
					Description: fmt.Sprintf("update saved query '%s': .%s → .%s", name, oldField, newField),
				})
			}
		}
	}

	if changedQueries {
		cfgOut, err := yaml.Marshal(vaultCfg)
		if err != nil {
			return nil, newError(ErrorInternal, err.Error(), "", nil, err)
		}
		plan.RavenYAML = cfgOut
	}

	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		relPath := result.RelativePath
		original := result.Document.RawContent
		lines := strings.Split(original, "\n")

		needsFrontmatterRename := false
		var frontmatterYAML map[string]interface{}
		startLine, endLine, fmOK := parser.FrontmatterBounds(lines)
		if fmOK && endLine != -1 {
			fmContent := strings.Join(lines[startLine+1:endLine], "\n")
			if err := yaml.Unmarshal([]byte(fmContent), &frontmatterYAML); err == nil {
				if frontmatterYAML == nil {
					frontmatterYAML = map[string]interface{}{}
				}
				if t, ok := frontmatterYAML["type"].(string); ok && t == typeName {
					_, oldPresent := frontmatterYAML[oldField]
					_, newPresent := frontmatterYAML[newField]
					if oldPresent && newPresent {
						plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
							FilePath:      relPath,
							ConflictType:  "frontmatter",
							Message:       fmt.Sprintf("frontmatter contains both '%s' and '%s'", oldField, newField),
							Line:          1,
							OldFieldFound: true,
							NewFieldFound: true,
						})
						return nil
					}
					if oldPresent {
						needsFrontmatterRename = true
					}
				}
			}
		}

		typeDeclsToEdit := make([]*parser.EmbeddedTypeInfo, 0)
		contentStartLine := 1
		bodyContent := original
		if fmOK && endLine != -1 {
			contentStartLine = (endLine + 1) + 1
			if endLine+1 < len(lines) {
				bodyContent = strings.Join(lines[endLine+1:], "\n")
			} else {
				bodyContent = ""
			}
		}

		astContent, err := parser.ExtractFromAST([]byte(bodyContent), contentStartLine)
		if err == nil && astContent != nil {
			for _, decl := range astContent.TypeDecls {
				if decl == nil || decl.TypeName != typeName {
					continue
				}
				_, oldPresent := decl.Fields[oldField]
				_, newPresent := decl.Fields[newField]
				if oldPresent && newPresent {
					plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
						FilePath:      relPath,
						ConflictType:  "embedded",
						Message:       fmt.Sprintf("embedded ::%s(...) contains both '%s' and '%s'", typeName, oldField, newField),
						Line:          decl.Line,
						OldFieldFound: true,
						NewFieldFound: true,
					})
					return nil
				}
				if oldPresent {
					typeDeclsToEdit = append(typeDeclsToEdit, decl)
				}
			}
		}

		if !needsFrontmatterRename && len(typeDeclsToEdit) == 0 {
			return nil
		}

		modified := false
		if len(typeDeclsToEdit) > 0 {
			sort.Slice(typeDeclsToEdit, func(i, j int) bool {
				return typeDeclsToEdit[i].Line < typeDeclsToEdit[j].Line
			})
			for _, decl := range typeDeclsToEdit {
				if decl.Line <= 0 || decl.Line-1 >= len(lines) {
					return nil
				}
				declLine := lines[decl.Line-1]

				leadingSpace := ""
				for _, c := range declLine {
					if c == ' ' || c == '\t' {
						leadingSpace += string(c)
					} else {
						break
					}
				}

				newFields := make(map[string]schema.FieldValue, len(decl.Fields))
				for k, v := range decl.Fields {
					newFields[k] = v
				}
				newFields[newField] = newFields[oldField]
				delete(newFields, oldField)

				newDecl := leadingSpace + parser.SerializeTypeDeclaration(typeName, newFields)
				if newDecl != declLine {
					lines[decl.Line-1] = newDecl
					modified = true
					plan.Changes = append(plan.Changes, FieldRenameChange{
						FilePath:    relPath,
						ChangeType:  "embedded",
						Description: fmt.Sprintf("rename field '%s' → '%s' inside ::%s(...)", oldField, newField, typeName),
						Line:        decl.Line,
					})
				}
			}
		}

		updatedLines := lines
		if needsFrontmatterRename && fmOK && endLine != -1 {
			fmContent := strings.Join(strings.Split(original, "\n")[startLine+1:endLine], "\n")
			var fmMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(fmContent), &fmMap); err == nil {
				if fmMap == nil {
					fmMap = map[string]interface{}{}
				}
				if _, ok := fmMap[oldField]; ok {
					fmMap[newField] = fmMap[oldField]
					delete(fmMap, oldField)

					newFM, err := yaml.Marshal(fmMap)
					if err == nil {
						var b strings.Builder
						b.WriteString("---\n")
						b.Write(newFM)
						b.WriteString("---")

						if endLine+1 < len(updatedLines) {
							b.WriteString("\n")
							b.WriteString(strings.Join(updatedLines[endLine+1:], "\n"))
						}

						updated := b.String()
						plan.MarkdownFiles[result.Path] = []byte(updated)
						plan.Changes = append(plan.Changes, FieldRenameChange{
							FilePath:    relPath,
							ChangeType:  "frontmatter",
							Description: fmt.Sprintf("rename frontmatter key '%s:' → '%s:' for type '%s'", oldField, newField, typeName),
							Line:        1,
						})
						return nil
					}
				}
			}
		}

		if modified {
			plan.MarkdownFiles[result.Path] = []byte(strings.Join(updatedLines, "\n"))
		}
		return nil
	})
	if err != nil {
		return nil, newError(ErrorInternal, err.Error(), "", nil, err)
	}

	return plan, nil
}

func looksLikeTemplatePath(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") {
		return true
	}
	if strings.HasSuffix(s, ".md") {
		return true
	}
	if strings.HasPrefix(s, "templates") {
		return true
	}
	if strings.Contains(s, "\n") {
		return false
	}
	matched, _ := regexp.MatchString(`^[\w.-]+$`, s)
	return matched && len(s) < 100
}

func suggestRenamedDefaultPath(oldDefaultPath, oldName, newName string) (string, bool) {
	normalized := paths.NormalizeDirRoot(oldDefaultPath)
	if normalized == "" {
		return "", false
	}
	trimmed := strings.TrimSuffix(normalized, "/")
	if trimmed == "" {
		return "", false
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	parent := ""
	base := trimmed
	if lastSlash >= 0 {
		parent = trimmed[:lastSlash]
		base = trimmed[lastSlash+1:]
	}

	newBase := ""
	switch base {
	case oldName:
		newBase = newName
	case oldName + "s":
		newBase = newName + "s"
	default:
		return "", false
	}

	next := newBase
	if parent != "" {
		next = parent + "/" + newBase
	}
	next = paths.NormalizeDirRoot(next)
	if next == normalized {
		return "", false
	}
	return next, true
}

func planTypeDirectoryMove(relPath, newName string, plan *typeDefaultPathRenamePlan, vaultCfg *config.VaultConfig) (typeDirectoryMove, bool) {
	if plan == nil || vaultCfg == nil {
		return typeDirectoryMove{}, false
	}
	sourceRel := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	sourceID := vaultCfg.FilePathToObjectID(sourceRel)
	if !strings.HasPrefix(sourceID, plan.OldDefaultPath) {
		return typeDirectoryMove{}, false
	}
	suffix := strings.TrimPrefix(sourceID, plan.OldDefaultPath)
	if suffix == "" {
		return typeDirectoryMove{}, false
	}
	destID := plan.NewDefaultPath + suffix
	destRel := filepath.ToSlash(vaultCfg.ObjectIDToFilePath(destID, newName))
	if sourceRel == destRel {
		return typeDirectoryMove{}, false
	}
	return typeDirectoryMove{
		SourceRelPath:      sourceRel,
		DestinationRelPath: destRel,
		SourceID:           sourceID,
		DestinationID:      destID,
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
		destAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		sources[filepath.Clean(sourceAbs)] = struct{}{}

		if _, err := os.Stat(sourceAbs); err != nil {
			return fmt.Errorf("source file does not exist: %s", move.SourceRelPath)
		}
		if existingSource, exists := destinations[filepath.Clean(destAbs)]; exists && existingSource != move.SourceRelPath {
			return fmt.Errorf("multiple files would move to '%s'", move.DestinationRelPath)
		}
		destinations[filepath.Clean(destAbs)] = move.SourceRelPath
	}

	for _, move := range moves {
		destAbs := filepath.Clean(filepath.Join(vaultPath, move.DestinationRelPath))
		if _, isSource := sources[destAbs]; isSource {
			continue
		}
		if _, err := os.Stat(destAbs); err == nil {
			return fmt.Errorf("destination already exists: %s", move.DestinationRelPath)
		}
	}

	return nil
}

func applyTypeDirectoryRename(vaultPath string, vaultCfg *config.VaultConfig, moves []typeDirectoryMove) (int, int, error) {
	if len(moves) == 0 {
		return 0, 0, nil
	}

	orderedMoves := make([]typeDirectoryMove, len(moves))
	copy(orderedMoves, moves)
	sort.SliceStable(orderedMoves, func(i, j int) bool {
		return len(orderedMoves[i].SourceRelPath) > len(orderedMoves[j].SourceRelPath)
	})

	idMoves := make(map[string]string, len(orderedMoves))
	for _, move := range orderedMoves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destAbs := filepath.Join(vaultPath, move.DestinationRelPath)

		if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
			return 0, 0, err
		}
		if err := os.Rename(sourceAbs, destAbs); err != nil {
			return 0, 0, err
		}
		idMoves[move.SourceID] = move.DestinationID
	}

	updatedReferenceFiles, err := updateReferencesForTypeDirectoryMoves(vaultPath, vaultCfg, idMoves)
	if err != nil {
		return len(orderedMoves), updatedReferenceFiles, err
	}
	return len(orderedMoves), updatedReferenceFiles, nil
}

func updateReferencesForTypeDirectoryMoves(vaultPath string, vaultCfg *config.VaultConfig, idMoves map[string]string) (int, error) {
	if len(idMoves) == 0 {
		return 0, nil
	}

	objectRoot := ""
	pageRoot := ""
	if vaultCfg != nil {
		objectRoot = vaultCfg.GetObjectsRoot()
		pageRoot = vaultCfg.GetPagesRoot()
	}

	oldIDs := make([]string, 0, len(idMoves))
	for oldID := range idMoves {
		oldIDs = append(oldIDs, oldID)
	}
	sort.SliceStable(oldIDs, func(i, j int) bool {
		return len(oldIDs[i]) > len(oldIDs[j])
	})

	updatedFiles := 0
	err := vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Path == "" {
			return nil
		}

		content, readErr := os.ReadFile(result.Path)
		if readErr != nil {
			return readErr
		}

		original := string(content)
		updated := original
		for _, oldID := range oldIDs {
			updated = objectsvc.ReplaceAllRefVariants(updated, oldID, oldID, idMoves[oldID], objectRoot, pageRoot)
		}

		if updated == original {
			return nil
		}
		if err := atomicfile.WriteFile(result.Path, []byte(updated), 0o644); err != nil {
			return err
		}
		updatedFiles++
		return nil
	})
	if err != nil {
		return updatedFiles, err
	}
	return updatedFiles, nil
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
