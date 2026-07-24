package checkfixsvc

import (
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
)

type CreateMissingResult struct {
	Created  int                    `json:"created"`
	Failures []CreateMissingFailure `json:"failures,omitempty"`
}

type CreateMissingFailure struct {
	TargetPath string `json:"target_path"`
	TypeName   string `json:"type_name,omitempty"`
	Error      string `json:"error"`
}

func CreateMissingRefsNonInteractive(
	vaultPath string,
	sch *schema.Schema,
	refs []*check.MissingRef,
	objectsRoot string,
	pagesRoot string,
	dailyDir string,
	templateDir string,
	protectedPrefixes []string,
) CreateMissingResult {
	result := CreateMissingResult{}
	seen := make(map[string]struct{})

	for _, ref := range refs {
		typeName := ref.InferredType
		if typeName == "" {
			continue
		}
		if _, exists := sch.Types[typeName]; !exists && !schema.IsBuiltinType(typeName) {
			continue
		}

		resolvedPath := ResolveTargetPath(ref.TargetPath, typeName, sch, objectsRoot, pagesRoot, dailyDir)
		slugPath := slugs.PathSlug(resolvedPath)
		if _, alreadyHandled := seen[slugPath]; alreadyHandled {
			continue
		}
		seen[slugPath] = struct{}{}

		if pages.Exists(vaultPath, resolvedPath) {
			continue
		}

		err := CreateMissingPage(vaultPath, sch, ref.TargetPath, typeName, objectsRoot, pagesRoot, dailyDir, templateDir, protectedPrefixes)
		if err != nil {
			result.Failures = append(result.Failures, CreateMissingFailure{
				TargetPath: ref.TargetPath,
				TypeName:   typeName,
				Error:      err.Error(),
			})
			continue
		}
		result.Created++
	}

	return result
}
