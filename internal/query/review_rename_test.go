package query

import (
	"testing"
)

func TestFilterRenamePairs(t *testing.T) {
	tests := []struct {
		name     string
		findings []ReviewFinding
		wantLen  int
	}{
		{
			name:     "empty findings",
			findings: nil,
			wantLen:  0,
		},
		{
			name: "rename pair filtered out",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Function Foo removed", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Function Bar added", RuleID: "ckb/breaking/function"},
			},
			wantLen: 1, // only the "added" survives
		},
		{
			name: "removal without matching add kept",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Function Foo removed", RuleID: "ckb/breaking/function"},
			},
			wantLen: 1,
		},
		{
			name: "different files not paired",
			findings: []ReviewFinding{
				{File: "a.go", Message: "Type X removed", RuleID: "ckb/breaking/type"},
				{File: "b.go", Message: "Type Y added", RuleID: "ckb/breaking/type"},
			},
			wantLen: 2,
		},
		{
			name: "different kinds not paired",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Function Foo removed", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Type Bar added", RuleID: "ckb/breaking/type"},
			},
			wantLen: 2,
		},
		{
			name: "multiple renames in same file",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Function A removed", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Function B removed", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Function C added", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Function D added", RuleID: "ckb/breaking/function"},
			},
			wantLen: 2, // both "added" survive, both "removed" paired away
		},
		{
			name: "case variation Removed/Added",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Removed function Foo", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "Added function Bar", RuleID: "ckb/breaking/function"},
			},
			wantLen: 1,
		},
		{
			name: "new keyword also matches as add",
			findings: []ReviewFinding{
				{File: "api.go", Message: "Function Foo removed", RuleID: "ckb/breaking/function"},
				{File: "api.go", Message: "new function Bar", RuleID: "ckb/breaking/function"},
			},
			wantLen: 1,
		},
		{
			name: "non-breaking findings pass through",
			findings: []ReviewFinding{
				{File: "api.go", Message: "complexity increased", RuleID: "ckb/complexity"},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterRenamePairs(tt.findings)
			if len(got) != tt.wantLen {
				t.Errorf("filterRenamePairs() returned %d findings, want %d\nfindings: %+v",
					len(got), tt.wantLen, got)
			}
		})
	}
}
