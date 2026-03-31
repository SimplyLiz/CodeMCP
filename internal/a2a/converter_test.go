package a2a

import (
	"testing"
)

func TestParseSkillRequest(t *testing.T) {
	tests := []struct {
		name      string
		msg       Message
		wantSkill string
		wantErr   bool
	}{
		{
			name: "valid JSON skill request",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{{Text: `{"skill": "searchSymbols", "params": {"query": "handleAuth"}}`}},
			},
			wantSkill: "searchSymbols",
		},
		{
			name: "valid with extra whitespace",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{{Text: `  {"skill": "getStatus", "params": {}}  `}},
			},
			wantSkill: "getStatus",
		},
		{
			name: "no skill in JSON",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{{Text: `{"params": {"query": "test"}}`}},
			},
			wantErr: true,
		},
		{
			name: "plain text (not JSON)",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{{Text: "search for handleAuth"}},
			},
			wantErr: true,
		},
		{
			name: "empty message",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{},
			},
			wantErr: true,
		},
		{
			name: "skill in second part",
			msg: Message{
				Role: RoleUser,
				Parts: []Part{
					{Text: "some preamble"},
					{Text: `{"skill": "getArchitecture", "params": {}}`},
				},
			},
			wantSkill: "getArchitecture",
		},
		{
			name: "missing params defaults to empty map",
			msg: Message{
				Role:  RoleUser,
				Parts: []Part{{Text: `{"skill": "getStatus"}`}},
			},
			wantSkill: "getStatus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, params, err := ParseSkillRequest(tt.msg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if skill != tt.wantSkill {
				t.Errorf("skill = %s, want %s", skill, tt.wantSkill)
			}
			if params == nil {
				t.Error("params should not be nil")
			}
		})
	}
}

func TestEnvelopeToArtifacts_Nil(t *testing.T) {
	arts := EnvelopeToArtifacts(nil, "test")
	if arts != nil {
		t.Errorf("expected nil for nil envelope, got %d artifacts", len(arts))
	}
}
