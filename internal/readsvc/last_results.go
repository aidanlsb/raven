package readsvc

import (
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
)

func SaveSearchResults(vaultPath, query string, results []model.SearchMatch) {
	modelResults := make([]model.Result, len(results))
	for i, result := range results {
		modelResults[i] = result
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceSearch, query, "", modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func SaveBacklinksResults(vaultPath, target string, links []model.Reference) {
	modelResults := make([]model.Result, len(links))
	for i, link := range links {
		modelResults[i] = link
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceBacklinks, "", target, modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func SaveOutlinksResults(vaultPath, source string, links []model.Reference) {
	modelResults := make([]model.Result, len(links))
	for i, link := range links {
		modelResults[i] = link
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceOutlinks, "", source, modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func SaveObjectQueryResults(vaultPath, queryString string, results []model.Object) {
	modelResults := make([]model.Result, len(results))
	for i, result := range results {
		modelResults[i] = result
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceQuery, queryString, "", modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func SaveTraitQueryResults(vaultPath, queryString string, results []model.Trait) {
	modelResults := make([]model.Result, len(results))
	for i, result := range results {
		modelResults[i] = result
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceQuery, queryString, "", modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}
