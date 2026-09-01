// Package cli implements the command-line interface.
package cli

import "github.com/aidanlsb/raven/internal/codes"

// Compatibility aliases used by the remaining CLI-only paths. The canonical
// definitions and complete stable-code inventory live in internal/codes.
const (
	ErrVaultNotFound     = codes.ErrVaultNotFound
	ErrVaultNotSpecified = codes.ErrVaultNotSpecified
	ErrConfigInvalid     = codes.ErrConfigInvalid

	ErrSchemaInvalid = codes.ErrSchemaInvalid
	ErrTypeNotFound  = codes.ErrTypeNotFound

	ErrRefNotFound  = codes.ErrRefNotFound
	ErrRefAmbiguous = codes.ErrRefAmbiguous

	ErrFileNotFound = codes.ErrFileNotFound
	ErrFileExists   = codes.ErrFileExists
	ErrFileRead     = codes.ErrFileRead
	ErrFileWrite    = codes.ErrFileWrite
	ErrDatabase     = codes.ErrDatabase

	ErrRequiredFieldMissing = codes.ErrRequiredFieldMissing

	ErrInvalidInput    = codes.ErrInvalidInput
	ErrMissingArgument = codes.ErrMissingArgument

	ErrMCPClientInvalid = codes.ErrMCPClientInvalid
	ErrMCPConfigWrite   = codes.ErrMCPConfigWrite

	ErrInternal = codes.ErrInternal

	ErrConfirmationRequired = codes.ErrConfirmationRequired
)

const WarnSectionSkipped = codes.WarnSectionSkipped

// mapCodeOrInternal returns code unchanged for known translations, or
// codes.ErrInternal for unrecognized codes. Preserves INVALID_ARGS →
// INVALID_INPUT; all other known codes pass through.
func mapCodeOrInternal(code codes.ErrorCode) codes.ErrorCode {
	switch code {
	case codes.ErrInvalidArgs:
		return codes.ErrInvalidInput
	case codes.ErrConfigInvalid,
		codes.ErrRefAmbiguous,
		codes.ErrRefNotFound,
		codes.ErrInvalidInput,
		codes.ErrDatabase,
		codes.ErrFileNotFound,
		codes.ErrFileRead,
		codes.ErrMissingArgument:
		return code
	default:
		return codes.ErrInternal
	}
}
