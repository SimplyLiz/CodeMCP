package compression

import "ckb/internal/config"

// ResponseBudget defines limits for response sizes to keep token counts manageable
type ResponseBudget struct {
	// MaxModules limits the number of modules returned in a single response
	MaxModules int

	// MaxSymbolsPerModule limits symbols shown per module
	MaxSymbolsPerModule int

	// MaxImpactItems limits the number of impact items returned
	MaxImpactItems int

	// MaxDrilldowns limits the number of suggested follow-up queries
	MaxDrilldowns int

	// EstimatedMaxTokens is the estimated maximum token count for the response
	EstimatedMaxTokens int
}

// DefaultBudget returns the default response budget with conservative limits
func DefaultBudget() *ResponseBudget {
	return &ResponseBudget{
		MaxModules:          10,
		MaxSymbolsPerModule: 5,
		MaxImpactItems:      20,
		MaxDrilldowns:       5,
		EstimatedMaxTokens:  4000,
	}
}

// LoadFromConfig creates a ResponseBudget from configuration, using defaults for missing values
func (b *ResponseBudget) LoadFromConfig(cfg *config.Config) *ResponseBudget {
	if cfg == nil {
		return DefaultBudget()
	}

	budget := &ResponseBudget{
		MaxModules:          cfg.Budget.MaxModules,
		MaxSymbolsPerModule: cfg.Budget.MaxSymbolsPerModule,
		MaxImpactItems:      cfg.Budget.MaxImpactItems,
		MaxDrilldowns:       cfg.Budget.MaxDrilldowns,
		EstimatedMaxTokens:  cfg.Budget.EstimatedMaxTokens,
	}

	// Apply defaults for zero values
	if budget.MaxModules == 0 {
		budget.MaxModules = 10
	}
	if budget.MaxSymbolsPerModule == 0 {
		budget.MaxSymbolsPerModule = 5
	}
	if budget.MaxImpactItems == 0 {
		budget.MaxImpactItems = 20
	}
	if budget.MaxDrilldowns == 0 {
		budget.MaxDrilldowns = 5
	}
	if budget.EstimatedMaxTokens == 0 {
		budget.EstimatedMaxTokens = 4000
	}

	return budget
}

// NewBudgetFromConfig creates a new ResponseBudget from a config file
func NewBudgetFromConfig(cfg *config.Config) *ResponseBudget {
	return DefaultBudget().LoadFromConfig(cfg)
}
