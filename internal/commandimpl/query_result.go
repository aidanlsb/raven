package commandimpl

import (
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/querysvc"
)

func shapeQueryResult(result *querysvc.ExecuteResult, options queryResultShapeOptions) commandexec.Result {
	data, count := querysvc.Envelope(result, querysvc.EnvelopeOptions{
		IDsOnly:        options.IDsOnly,
		CountOnly:      options.CountOnly,
		IsSavedQuery:   options.IsSavedQuery,
		SavedQueryName: options.SavedQueryName,
	})
	return commandexec.Success(data, &commandexec.Meta{Count: count, QueryTimeMs: options.QueryTimeMs})
}

type queryResultShapeOptions struct {
	IDsOnly        bool
	CountOnly      bool
	IsSavedQuery   bool
	SavedQueryName string
	QueryTimeMs    int64
}
