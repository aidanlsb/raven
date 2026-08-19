package schemasvc

import (
	"errors"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func editSchema(vaultPath, loadSuggestion string, mutate func(*schemadoc.Document) error) error {
	return editSchemaWithLoadError(vaultPath, loadSuggestion, codes.ErrSchemaNotFound, mutate)
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
