// Package schemamigratesvc orchestrates vault-wide migrations that follow schema
// changes. Schema document transformations remain in schemasvc; this package
// stages and applies the corresponding config, template, Markdown, reference,
// and path updates.
package schemamigratesvc

import (
	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemachange"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// writeSchemaWithInvalidation writes schema.yaml and records invalidation for
// auto-reindex. It returns the operation ID and classification for later
// application via schemachange.ApplyInvalidation.
func writeSchemaWithInvalidation(rt *vaultruntime.Runtime, schemaBytes []byte) (string, schemachange.Classification, error) {
	vaultPath := rt.VaultPath

	beforeSchema, _ := schema.Load(vaultPath)

	afterResult, parseErr := schema.Parse(schemaBytes, paths.SchemaPath(vaultPath))
	if parseErr != nil {
		return "", schemachange.Classification{}, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to parse staged schema", parseErr)
	}
	afterSchema := afterResult.Schema

	operationID, classification, err := schemachange.RecordInvalidation(vaultPath, beforeSchema, afterSchema)
	if err != nil {
		return "", schemachange.Classification{}, svcerr.Wrap(codes.ErrInternal, "failed to record schema invalidation", err)
	}

	if err := schemadoc.Write(vaultPath, schemaBytes); err != nil {
		return "", schemachange.Classification{}, schemasvc.MapSchemaDocError(err, "", codes.ErrSchemaNotFound)
	}

	return operationID, classification, nil
}

type stagedApply struct {
	SchemaYAML      []byte
	SchemaApplied   int
	FileSets        []map[string][]byte
	WriteSuggestion string
	ReloadContext   string
	AfterFiles      func() (int, error)
}

func applyStagedFilesThenInvalidate(rt *vaultruntime.Runtime, apply stagedApply) (int, error) {
	applied := 0
	var operationID string
	var classification schemachange.Classification

	if len(apply.SchemaYAML) > 0 {
		opID, classif, err := writeSchemaWithInvalidation(rt, apply.SchemaYAML)
		if err != nil {
			return 0, err
		}
		operationID = opID
		classification = classif
		applied += apply.SchemaApplied
	}

	n, err := writeStagedFileMaps(apply.WriteSuggestion, apply.FileSets...)
	if err != nil {
		return 0, err
	}
	applied += n

	if apply.AfterFiles != nil {
		extra, afterErr := apply.AfterFiles()
		if afterErr != nil {
			return 0, afterErr
		}
		applied += extra
	}

	if operationID != "" {
		if err := rt.ReloadSchema(true); err != nil {
			return 0, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to reload schema after "+apply.ReloadContext, err)
		}
		// Attempt to apply invalidation. If it fails, the schema write still succeeded
		// and the journal entry persists, so a manual reindex will recover.
		_ = schemachange.ApplyInvalidation(rt, operationID, classification, readsvc.ReindexForSchemaChange)
	}

	return applied, nil
}

func writeStagedFileMaps(writeSuggestion string, fileSets ...map[string][]byte) (int, error) {
	applied := 0
	for _, files := range fileSets {
		for _, path := range schemasvc.SortedKeys(files) {
			if err := atomicfile.WriteFile(path, files[path], 0o644); err != nil {
				wrapped := svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
				if writeSuggestion != "" {
					wrapped = wrapped.WithSuggestion(writeSuggestion)
				}
				return 0, wrapped
			}
			applied++
		}
	}
	return applied, nil
}

func loadSchemaDocument(vaultPath string) (*schemadoc.Document, error) {
	doc, err := schemadoc.Load(vaultPath)
	if err != nil {
		return nil, schemasvc.MapSchemaDocError(err, "Run 'rvn init' first", codes.ErrSchemaNotFound)
	}
	return doc, nil
}
