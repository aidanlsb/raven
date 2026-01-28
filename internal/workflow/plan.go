package workflow

import (
	"encoding/json"
	"fmt"
)

// PromptEnvelope is the required JSON envelope for prompt step outputs.
//
// Example:
// { "outputs": { "markdown": "...", "plan": { "plan_version": 1, "ops": [...] } } }
type PromptEnvelope struct {
	Outputs map[string]json.RawMessage `json:"outputs"`
}

// Plan is a deterministic patch plan emitted by an agent.
type Plan struct {
	PlanVersion int  `json:"plan_version"`
	Ops         []Op `json:"ops"`
}

type Op struct {
	Op   string          `json:"op"`
	Why  string          `json:"why"`
	Args json.RawMessage `json:"args"`
}

type AddArgs struct {
	To      string `json:"to"`
	Heading string `json:"heading,omitempty"`
	Text    string `json:"text"`
}

type EditArgs struct {
	Path   string `json:"path"`
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

type SetArgs struct {
	ObjectID string            `json:"object_id"`
	Fields   map[string]string `json:"fields"`
}

type MoveArgs struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	UpdateRefs  *bool  `json:"update_refs,omitempty"`
}

type UpdateTraitArgs struct {
	TraitID string `json:"trait_id"`
	Value   string `json:"value"`
}

// ValidatePlan performs schema-level validation (not vault-specific semantics).
func ValidatePlan(p *Plan) error {
	if p == nil {
		return fmt.Errorf("plan is nil")
	}
	if p.PlanVersion != 1 {
		return fmt.Errorf("unsupported plan_version: %d", p.PlanVersion)
	}
	if p.Ops == nil {
		return fmt.Errorf("plan missing ops")
	}
	for i, op := range p.Ops {
		if op.Op == "" {
			return fmt.Errorf("op[%d] missing op", i)
		}
		if op.Why == "" {
			return fmt.Errorf("op[%d] missing why", i)
		}
		if len(op.Args) == 0 {
			return fmt.Errorf("op[%d] missing args", i)
		}
		switch op.Op {
		case "add", "edit", "set", "move", "update_trait":
		default:
			return fmt.Errorf("op[%d] has unknown op: %s", i, op.Op)
		}
	}
	return nil
}
