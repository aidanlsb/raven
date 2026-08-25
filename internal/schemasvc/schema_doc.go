package schemasvc

import (
	"errors"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schemachange"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// editSchemaResult extends schemadoc.EditResult with classification info.
type editSchemaResult struct {
	*schemadoc.EditResult
	Classification schemachange.Classification
}

func editSchema(vaultPath, loadSuggestion string, mutate func(*schemadoc.Document) error) error {
	return editSchemaWithLoadError(vaultPath, loadSuggestion, codes.ErrSchemaNotFound, mutate)
}

func editSchemaWithInvalidation(vaultPath, loadSuggestion string, mutate func(*schemadoc.Document) error) (*editSchemaResult, error) {
	return editSchemaWithInvalidationAndLoadError(vaultPath, loadSuggestion, codes.ErrSchemaNotFound, mutate)
}

func editSchemaWithLoadError(
	vaultPath string,
	loadSuggestion string,
	loadErrorCode codes.ErrorCode,
	mutate func(*schemadoc.Document) error,
) error {
	if err := schemadoc.Edit(vaultPath, mutate); err != nil {
		return MapSchemaDocError(err, loadSuggestion, loadErrorCode)
	}
	return nil
}

func editSchemaWithInvalidationAndLoadError(
	vaultPath string,
	loadSuggestion string,
	loadErrorCode codes.ErrorCode,
	mutate func(*schemadoc.Document) error,
) (*editSchemaResult, error) {
	result, err := schemadoc.EditWithInvalidation(vaultPath, mutate)
	if err != nil {
		return nil, MapSchemaDocError(err, loadSuggestion, loadErrorCode)
	}
	// Extract classification from the last edit
	classification := schemachange.Classification{Policy: schemachange.PolicyNone}
	if raw := schemadoc.GetLastClassification(); raw != nil {
		if c, ok := raw.(schemachange.Classification); ok {
			classification = c
		}
	}
	return &editSchemaResult{
		EditResult:     result,
		Classification: classification,
	}, nil
}

// MapSchemaDocError converts schemadoc failures to schemasvc's stable error
// codes so schema mutation orchestrators share the same response contract.
func MapSchemaDocError(err error, loadSuggestion string, loadErrorCode codes.ErrorCode) error {
	if err == nil {
		return nil
	}

	var docErr *schemadoc.Error
	if !errors.As(err, &docErr) {
		return err
	}

	switch docErr.Operation {
	case schemadoc.OperationRead:
		return svcerr.Wrap(codes.ErrFileRead, docErr.Error(), docErr)
	case schemadoc.OperationLoad:
		return svcerr.Wrap(loadErrorCode, docErr.Error(), docErr).WithSuggestion(loadSuggestion)
	case schemadoc.OperationDecode, schemadoc.OperationValidate:
		return svcerr.Wrap(codes.ErrSchemaInvalid, docErr.Error(), docErr)
	case schemadoc.OperationMarshal:
		return svcerr.Wrap(codes.ErrInternal, docErr.Error(), docErr)
	case schemadoc.OperationWrite:
		return svcerr.Wrap(codes.ErrFileWrite, docErr.Error(), docErr)
	default:
		return svcerr.Wrap(codes.ErrInternal, docErr.Error(), docErr)
	}
}
