package schemasvc

import (
	"errors"

	"github.com/aidanlsb/raven/internal/schemadoc"
)

func editSchema(vaultPath, loadSuggestion string, mutate func(*schemadoc.Document) error) error {
	return editSchemaWithLoadError(vaultPath, loadSuggestion, ErrorSchemaNotFound, mutate)
}

func editSchemaWithLoadError(
	vaultPath string,
	loadSuggestion string,
	loadErrorCode ErrorCode,
	mutate func(*schemadoc.Document) error,
) error {
	if err := schemadoc.Edit(vaultPath, mutate); err != nil {
		return mapSchemaDocError(err, loadSuggestion, loadErrorCode)
	}
	return nil
}

func loadSchemaDocument(vaultPath, loadSuggestion string) (*schemadoc.Document, error) {
	doc, err := schemadoc.Load(vaultPath)
	if err != nil {
		return nil, mapSchemaDocError(err, loadSuggestion, ErrorSchemaNotFound)
	}
	return doc, nil
}

func mapSchemaDocError(err error, loadSuggestion string, loadErrorCode ErrorCode) error {
	if err == nil {
		return nil
	}

	var docErr *schemadoc.Error
	if !errors.As(err, &docErr) {
		return err
	}

	switch docErr.Operation {
	case schemadoc.OperationRead:
		return newError(ErrorFileRead, docErr.Error(), "", nil, docErr)
	case schemadoc.OperationLoad:
		return newError(loadErrorCode, docErr.Error(), loadSuggestion, nil, docErr)
	case schemadoc.OperationDecode, schemadoc.OperationValidate:
		return newError(ErrorSchemaInvalid, docErr.Error(), "", nil, docErr)
	case schemadoc.OperationMarshal:
		return newError(ErrorInternal, docErr.Error(), "", nil, docErr)
	case schemadoc.OperationWrite:
		return newError(ErrorFileWrite, docErr.Error(), "", nil, docErr)
	default:
		return newError(ErrorInternal, docErr.Error(), "", nil, docErr)
	}
}
