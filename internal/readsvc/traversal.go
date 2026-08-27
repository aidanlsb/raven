package readsvc

import (
	"errors"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ReferenceFailure struct {
	Input     string
	Operation string
	Err       error
}

type BacklinksResult struct {
	Groups   []model.BacklinksGroup
	Failures []ReferenceFailure
	Total    int
}

type OutlinksResult struct {
	Groups   []model.OutlinksGroup
	Failures []ReferenceFailure
	Total    int
}

type ResolveResult struct {
	Resolved     *refresolve.ResolveResult
	ObjectType   string
	Ambiguous    bool
	Matches      []string
	MatchSources map[string]string
}

func BacklinksForReferences(rt *vaultruntime.Runtime, references []string) BacklinksResult {
	result := BacklinksResult{Groups: make([]model.BacklinksGroup, 0, len(references))}
	for _, reference := range references {
		resolved, err := refresolve.ResolveDynamic(reference, rt, true)
		if err != nil {
			result.Failures = append(result.Failures, ReferenceFailure{Input: reference, Operation: "resolve", Err: err})
			continue
		}
		links, err := Backlinks(rt, resolved.ObjectID)
		if err != nil {
			result.Failures = append(result.Failures, ReferenceFailure{Input: reference, Operation: "backlinks", Err: err})
			continue
		}
		result.Groups = append(result.Groups, model.BacklinksGroup{
			Input: reference, Target: resolved.ObjectID, Items: links, Count: len(links),
		})
		result.Total += len(links)
	}
	return result
}

func OutlinksForReferences(rt *vaultruntime.Runtime, references []string) OutlinksResult {
	result := OutlinksResult{Groups: make([]model.OutlinksGroup, 0, len(references))}
	for _, reference := range references {
		resolved, err := refresolve.ResolveDynamic(reference, rt, true)
		if err != nil {
			result.Failures = append(result.Failures, ReferenceFailure{Input: reference, Operation: "resolve", Err: err})
			continue
		}
		links, err := Outlinks(rt, resolved.ObjectID)
		if err != nil {
			result.Failures = append(result.Failures, ReferenceFailure{Input: reference, Operation: "outlinks", Err: err})
			continue
		}
		result.Groups = append(result.Groups, model.OutlinksGroup{
			Input: reference, Source: resolved.ObjectID, Items: links, Count: len(links),
		})
		result.Total += len(links)
	}
	return result
}

func ResolveReference(rt *vaultruntime.Runtime, reference string) (*ResolveResult, error) {
	resolved, err := refresolve.ResolveDynamic(reference, rt, true)
	var ambiguous *refresolve.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		return &ResolveResult{
			Ambiguous: true, Matches: ambiguous.Matches, MatchSources: ambiguous.MatchSources,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	result := &ResolveResult{Resolved: resolved}
	if rt != nil && rt.DB != nil {
		if object, objectErr := rt.DB.GetObject(resolved.ObjectID); objectErr == nil && object != nil {
			result.ObjectType = object.Type
		}
	}
	return result, nil
}
