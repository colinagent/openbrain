package op

import "testing"

func TestMetaClone_PreservesStructuredValues(t *testing.T) {
	original := Meta{
		"selectedSkillIDs": []any{"skill-plan"},
		"selectedSkillContext": map[string]any{
			"planFilePath": "/tmp/demo/.agent/plan/thread-1.plan.md",
		},
	}

	cloned := original.Clone()
	ids, ok := cloned["selectedSkillIDs"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "skill-plan" {
		t.Fatalf("cloned selectedSkillIDs = %#v", cloned["selectedSkillIDs"])
	}
	ctx, ok := cloned["selectedSkillContext"].(map[string]any)
	if !ok || ctx["planFilePath"] != "/tmp/demo/.agent/plan/thread-1.plan.md" {
		t.Fatalf("cloned selectedSkillContext = %#v", cloned["selectedSkillContext"])
	}

	ids[0] = "changed"
	ctx["planFilePath"] = "changed"
	origIDs := original["selectedSkillIDs"].([]any)
	origContext := original["selectedSkillContext"].(map[string]any)
	if origIDs[0] != "skill-plan" {
		t.Fatalf("original selectedSkillIDs mutated = %#v", original["selectedSkillIDs"])
	}
	if origContext["planFilePath"] != "/tmp/demo/.agent/plan/thread-1.plan.md" {
		t.Fatalf("original selectedSkillContext mutated = %#v", original["selectedSkillContext"])
	}
}
