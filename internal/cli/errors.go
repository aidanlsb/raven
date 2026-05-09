// Package cli implements the command-line interface.
package cli

import "github.com/aidanlsb/raven/internal/codes"

// Error codes for structured error responses.
// These codes are stable and can be relied upon by agents.
const (
	// Vault errors
	ErrVaultNotFound     = codes.ErrVaultNotFound
	ErrVaultNotSpecified = codes.ErrVaultNotSpecified
	ErrConfigInvalid     = codes.ErrConfigInvalid

	// Schema errors
	ErrSchemaNotFound = codes.ErrSchemaNotFound
	ErrSchemaInvalid  = codes.ErrSchemaInvalid
	ErrTypeNotFound   = codes.ErrTypeNotFound
	ErrTraitNotFound  = codes.ErrTraitNotFound
	ErrFieldNotFound  = codes.ErrFieldNotFound

	// Object errors
	ErrObjectNotFound = codes.ErrObjectNotFound
	ErrObjectExists   = codes.ErrObjectExists
	ErrObjectInvalid  = codes.ErrObjectInvalid

	// Reference errors
	ErrRefNotFound  = codes.ErrRefNotFound
	ErrRefInvalid   = codes.ErrRefInvalid
	ErrRefAmbiguous = codes.ErrRefAmbiguous

	// File errors
	ErrFileNotFound     = codes.ErrFileNotFound
	ErrFileExists       = codes.ErrFileExists
	ErrFileReadError    = codes.ErrFileRead
	ErrFileWriteError   = codes.ErrFileWrite
	ErrFileOutsideVault = codes.ErrFileOutsideVault

	// Database errors
	ErrDatabaseError   = codes.ErrDatabase
	ErrDatabaseVersion = codes.ErrDatabaseVersion

	// Validation errors
	ErrValidationFailed     = codes.ErrValidationFailed
	ErrRequiredFieldMissing = codes.ErrRequiredFieldMissing
	ErrInvalidValue         = codes.ErrInvalidValue
	ErrUnknownField         = codes.ErrUnknownField

	// Query errors
	ErrQueryNotFound = codes.ErrQueryNotFound
	ErrQueryInvalid  = codes.ErrQueryInvalid
	ErrDuplicateName = codes.ErrDuplicateName

	// Input errors
	ErrInvalidInput    = codes.ErrInvalidInput
	ErrInvalidArgs     = codes.ErrInvalidArgs
	ErrMissingArgument = codes.ErrMissingArgument
	ErrCommandNotFound = codes.ErrCommandNotFound

	// Skill errors
	ErrSkillNotFound          = codes.ErrSkillNotFound
	ErrSkillNotInstalled      = codes.ErrSkillNotInstalled
	ErrSkillTargetUnsupported = codes.ErrSkillTargetUnsupported
	ErrSkillRenderFailed      = codes.ErrSkillRenderFailed
	ErrSkillPathUnresolved    = codes.ErrSkillPathUnresolved
	ErrSkillReceiptInvalid    = codes.ErrSkillReceiptInvalid

	// MCP client errors
	ErrMCPClientInvalid    = codes.ErrMCPClientInvalid
	ErrMCPConfigWriteError = codes.ErrMCPConfigWrite

	// General errors
	ErrInternal       = codes.ErrInternal
	ErrNotImplemented = codes.ErrNotImplemented

	// Schema modification errors
	ErrDataIntegrityBlock   = codes.ErrDataIntegrityBlock
	ErrConfirmationRequired = codes.ErrConfirmationRequired
)

// Warning codes for non-fatal issues.
const (
	WarnRefNotFound       = codes.WarnRefNotFound
	WarnDeprecated        = codes.WarnDeprecated
	WarnSchemaOutdated    = codes.WarnSchemaOutdated
	WarnDatabaseOutdated  = codes.WarnDatabaseOutdated
	WarnIndexUpdateFailed = codes.WarnIndexUpdateFailed
	WarnDocsFetchFailed   = codes.WarnDocsFetchFailed
	WarnWrongCommand      = codes.WarnWrongCommand
	WarnMissingField      = codes.WarnMissingField
	WarnBacklinks         = codes.WarnBacklinks
	WarnEmbeddedSkipped   = codes.WarnEmbeddedSkipped
	WarnUnknownField      = codes.WarnUnknownField
	WarnTypeMismatch      = codes.WarnTypeMismatch
)
