package query

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
)

func TestExecuteString_ObjectQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	result, err := exec.Execute("type:project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	objects, ok := result.([]model.Object)
	if !ok {
		t.Fatalf("expected []model.Object, got %T", result)
	}
	if len(objects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(objects))
	}
}

func TestExecuteString_TraitQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	result, err := exec.Execute("trait:due")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	traits, ok := result.([]model.Trait)
	if !ok {
		t.Fatalf("expected []model.Trait, got %T", result)
	}
	if len(traits) != 3 {
		t.Errorf("expected 3 traits, got %d", len(traits))
	}
}

func TestExecuteString_ParseError(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	_, err := exec.Execute("type:project .status==")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("expected error to contain 'parse error', got: %v", err)
	}
}

func TestExecuteString_MatchesManualParseThenExecute(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)
	queryStr := `type:project .status==active`

	directResult, err := exec.Execute(queryStr)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	q, err := Parse(queryStr)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	manualResult, err := exec.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("ExecuteObjectQuery: %v", err)
	}

	directObjects := directResult.([]model.Object)
	if len(directObjects) != len(manualResult) {
		t.Errorf("result count mismatch: Execute=%d, manual=%d", len(directObjects), len(manualResult))
	}

	for i := range directObjects {
		if i >= len(manualResult) {
			break
		}
		if directObjects[i].ID != manualResult[i].ID {
			t.Errorf("result %d: ID mismatch: %q vs %q", i, directObjects[i].ID, manualResult[i].ID)
		}
	}
}
