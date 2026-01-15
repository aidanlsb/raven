// Package cli implements the command-line interface.
package cli

// Error codes for structured error responses.
// These codes are stable and can be relied upon by agents.
const (
	// Vault errors
	ErrVaultNotFound     = "VAULT_NOT_FOUND"
	ErrVaultNotSpecified = "VAULT_NOT_SPECIFIED"

	// Schema errors
	ErrSchemaNotFound = "SCHEMA_NOT_FOUND"
	ErrSchemaInvalid  = "SCHEMA_INVALID"
	ErrTypeNotFound   = "TYPE_NOT_FOUND"
	ErrTraitNotFound  = "TRAIT_NOT_FOUND"
	ErrFieldNotFound  = "FIELD_NOT_FOUND"

	// Object errors
	ErrObjectNotFound = "OBJECT_NOT_FOUND"
	ErrObjectExists   = "OBJECT_EXISTS"
	ErrObjectInvalid  = "OBJECT_INVALID"

	// Reference errors
	ErrRefNotFound  = "REF_NOT_FOUND"
	ErrRefInvalid   = "REF_INVALID"
	ErrRefAmbiguous = "REF_AMBIGUOUS"

	// File errors
	ErrFileNotFound     = "FILE_NOT_FOUND"
	ErrFileExists       = "FILE_EXISTS"
	ErrFileReadError    = "FILE_READ_ERROR"
	ErrFileWriteError   = "FILE_WRITE_ERROR"
	ErrFileOutsideVault = "FILE_OUTSIDE_VAULT"

	// Database errors
	ErrDatabaseError   = "DATABASE_ERROR"
	ErrDatabaseVersion = "DATABASE_VERSION_MISMATCH"

	// Validation errors
	ErrValidationFailed = "VALIDATION_FAILED"
	ErrRequiredField    = "REQUIRED_FIELD_MISSING"
	ErrInvalidValue     = "INVALID_VALUE"
	ErrUnknownField     = "UNKNOWN_FIELD"

	// Query errors
	ErrQueryNotFound = "QUERY_NOT_FOUND"
	ErrQueryInvalid  = "QUERY_INVALID"
	ErrDuplicateName = "DUPLICATE_NAME"

	// Input errors
	ErrInvalidInput    = "INVALID_INPUT"
	ErrMissingArgument = "MISSING_ARGUMENT"

	// General errors
	ErrInternal       = "INTERNAL_ERROR"
	ErrNotImplemented = "NOT_IMPLEMENTED"

	// Schema modification errors
	ErrDataIntegrityBlock   = "DATA_INTEGRITY_BLOCK"
	ErrConfirmationRequired = "CONFIRMATION_REQUIRED"
	ErrFileDoesNotExist     = "FILE_DOES_NOT_EXIST"
	ErrRequiredFieldMissing = "REQUIRED_FIELD_MISSING"
)

// Warning codes for non-fatal issues.
const (
	WarnRefNotFound       = "REF_NOT_FOUND"
	WarnDeprecated        = "DEPRECATED"
	WarnSchemaOutdated    = "SCHEMA_OUTDATED"
	WarnDatabaseOutdated  = "DATABASE_OUTDATED"
	WarnIndexUpdateFailed = "INDEX_UPDATE_FAILED"
	WarnMisuse            = "WRONG_COMMAND"
	WarnMissingField      = "MISSING_REQUIRED_FIELD"
	WarnBacklinks         = "HAS_BACKLINKS"
	WarnUnknownField      = "UNKNOWN_FIELD"
	WarnTypeMismatch      = "TYPE_DIRECTORY_MISMATCH"
)
