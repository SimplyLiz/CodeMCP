// Package complexity provides language-agnostic complexity metrics via tree-sitter.
package complexity

// Language represents a supported programming language.
type Language string

const (
	LangGo         Language = "go"
	LangJavaScript Language = "javascript"
	LangTypeScript Language = "typescript"
	LangTSX        Language = "tsx"
	LangPython     Language = "python"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangKotlin     Language = "kotlin"
)

// ComplexityResult contains complexity metrics for a single unit (function/method).
type ComplexityResult struct {
	// Name is the function/method name
	Name string `json:"name"`

	// StartLine is the line number where the function starts
	StartLine int `json:"startLine"`

	// EndLine is the line number where the function ends
	EndLine int `json:"endLine"`

	// Cyclomatic is the cyclomatic complexity (decision points + 1)
	Cyclomatic int `json:"cyclomatic"`

	// Cognitive is the cognitive complexity (nested depth weighted)
	Cognitive int `json:"cognitive"`

	// Lines is the number of lines in the function
	Lines int `json:"lines"`
}

// FileComplexity contains complexity metrics for an entire file.
type FileComplexity struct {
	// Path is the file path
	Path string `json:"path"`

	// Language is the detected language
	Language Language `json:"language"`

	// Functions contains complexity for each function/method
	Functions []ComplexityResult `json:"functions"`

	// TotalCyclomatic is the sum of all function cyclomatic complexities
	TotalCyclomatic int `json:"totalCyclomatic"`

	// TotalCognitive is the sum of all function cognitive complexities
	TotalCognitive int `json:"totalCognitive"`

	// AverageCyclomatic is the average cyclomatic complexity
	AverageCyclomatic float64 `json:"averageCyclomatic"`

	// AverageCognitive is the average cognitive complexity
	AverageCognitive float64 `json:"averageCognitive"`

	// MaxCyclomatic is the highest cyclomatic complexity in the file
	MaxCyclomatic int `json:"maxCyclomatic"`

	// MaxCognitive is the highest cognitive complexity in the file
	MaxCognitive int `json:"maxCognitive"`

	// FunctionCount is the number of functions analyzed
	FunctionCount int `json:"functionCount"`

	// Error is set if analysis failed
	Error string `json:"error,omitempty"`
}

// Aggregate computes aggregate metrics from function results.
func (fc *FileComplexity) Aggregate() {
	fc.FunctionCount = len(fc.Functions)
	if fc.FunctionCount == 0 {
		return
	}

	for _, f := range fc.Functions {
		fc.TotalCyclomatic += f.Cyclomatic
		fc.TotalCognitive += f.Cognitive

		if f.Cyclomatic > fc.MaxCyclomatic {
			fc.MaxCyclomatic = f.Cyclomatic
		}
		if f.Cognitive > fc.MaxCognitive {
			fc.MaxCognitive = f.Cognitive
		}
	}

	fc.AverageCyclomatic = float64(fc.TotalCyclomatic) / float64(fc.FunctionCount)
	fc.AverageCognitive = float64(fc.TotalCognitive) / float64(fc.FunctionCount)
}

// LanguageFromExtension returns the Language for a file extension.
func LanguageFromExtension(ext string) (Language, bool) {
	switch ext {
	case ".go":
		return LangGo, true
	case ".js", ".mjs", ".cjs":
		return LangJavaScript, true
	case ".ts", ".mts", ".cts":
		return LangTypeScript, true
	case ".tsx":
		return LangTSX, true
	case ".jsx":
		return LangJavaScript, true // JSX uses JS parser
	case ".py", ".pyw":
		return LangPython, true
	case ".rs":
		return LangRust, true
	case ".java":
		return LangJava, true
	case ".kt", ".kts":
		return LangKotlin, true
	default:
		return "", false
	}
}
