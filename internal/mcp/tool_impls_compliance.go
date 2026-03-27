package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
	// Register all framework check packages
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/ccpa"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/do178c"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/dora"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/euaiact"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/eucra"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/fda21cfr11"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/gdpr"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/hipaa"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iec61508"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iec62443"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iso26262"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iso27001"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iso27701"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/misra"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/nis2"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/nist80053"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/owaspasvs"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/pcidss"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/sbom"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/soc2"
	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/errors"
)

// toolAuditCompliance runs a regulatory compliance audit against selected frameworks.
func (s *MCPServer) toolAuditCompliance(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse frameworks (required)
	var frameworks []compliance.FrameworkID
	if v, ok := params["frameworks"].([]interface{}); ok {
		for _, f := range v {
			if fs, ok := f.(string); ok && fs != "" {
				frameworks = append(frameworks, compliance.FrameworkID(fs))
			}
		}
	} else if v, ok := params["frameworks"].(string); ok && v != "" {
		// Accept comma-separated string too
		for _, f := range strings.Split(v, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				frameworks = append(frameworks, compliance.FrameworkID(f))
			}
		}
	}
	if len(frameworks) == 0 {
		return nil, fmt.Errorf("frameworks parameter is required (e.g., [\"gdpr\", \"pci-dss\"] or \"all\")")
	}

	// Parse optional params
	scope := ""
	if v, ok := params["scope"].(string); ok {
		scope = v
	}

	minConfidence := 0.5
	if v, ok := params["minConfidence"].(float64); ok && v > 0 {
		minConfidence = v
	}

	silLevel := 2
	if v, ok := params["silLevel"].(float64); ok && v >= 1 && v <= 4 {
		silLevel = int(v)
	}

	failOn := "error"
	if v, ok := params["failOn"].(string); ok && v != "" {
		failOn = v
	}

	var checks []string
	if v, ok := params["checks"].([]interface{}); ok {
		for _, c := range v {
			if cs, ok := c.(string); ok {
				checks = append(checks, cs)
			}
		}
	}

	opts := compliance.AuditOptions{
		RepoRoot:      s.engine().GetRepoRoot(),
		Frameworks:    frameworks,
		Scope:         scope,
		MinConfidence: minConfidence,
		SILLevel:      silLevel,
		Checks:        checks,
		FailOn:        failOn,
	}

	report, err := compliance.RunAudit(ctx, opts, s.logger)
	if err != nil {
		return nil, errors.NewOperationError("compliance audit", err)
	}

	return NewToolResponse().Data(report).Build(), nil
}
