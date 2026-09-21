package schemamigratesvc

import (
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func finishValueConversion(
	rt *vaultruntime.Runtime,
	confirm bool,
	kind, name, typeName string,
	sourceType, targetType schema.FieldType,
	plan *valueConvertPlan,
) (*ConvertResult, error) {
	result := &ConvertResult{
		Preview:      !confirm,
		Kind:         kind,
		Name:         name,
		TypeName:     typeName,
		SourceType:   string(sourceType),
		TargetType:   string(targetType),
		TotalChanges: len(plan.Changes),
		Changes:      plan.Changes,
		Hint:         "Run with --confirm to apply changes",
	}
	if !confirm {
		return result, nil
	}

	applied, err := applyValueConvertPlan(rt, plan)
	if err != nil {
		return nil, err
	}
	result.Preview = false
	result.Changes = nil
	result.TotalChanges = 0
	result.ChangesApplied = applied
	result.Hint = "Run 'rvn reindex --full' to update the index, then run 'rvn check'"
	return result, nil
}

func applyValueConvertPlan(rt *vaultruntime.Runtime, plan *valueConvertPlan) (int, error) {
	schemaYAML := []byte(nil)
	schemaApplied := 0
	if plan.SchemaPlan.SchemaMutations > 0 {
		schemaYAML = plan.SchemaPlan.SchemaYAML
		schemaApplied = plan.SchemaPlan.SchemaMutations
	}
	return applyStagedFilesThenInvalidate(rt, stagedApply{
		SchemaYAML:      schemaYAML,
		SchemaApplied:   schemaApplied,
		FileSets:        []map[string][]byte{plan.MarkdownFiles},
		WriteSuggestion: "Some files may already be converted; review the vault and run 'rvn reindex --full'",
		ReloadContext:   "convert",
	})
}
