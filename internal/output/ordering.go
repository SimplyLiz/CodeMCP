package output

import (
	"sort"
)

// SortModules sorts modules by impactCount DESC, symbolCount DESC, moduleId ASC
func SortModules(modules []Module) {
	sort.SliceStable(modules, func(i, j int) bool {
		// Primary: impactCount DESC
		if modules[i].ImpactCount != modules[j].ImpactCount {
			return modules[i].ImpactCount > modules[j].ImpactCount
		}
		// Secondary: symbolCount DESC
		if modules[i].SymbolCount != modules[j].SymbolCount {
			return modules[i].SymbolCount > modules[j].SymbolCount
		}
		// Tertiary: moduleId ASC
		return modules[i].ModuleId < modules[j].ModuleId
	})
}

// SortSymbols sorts symbols by confidence DESC, refCount DESC, stableId ASC
func SortSymbols(symbols []Symbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		// Primary: confidence DESC
		if symbols[i].Confidence != symbols[j].Confidence {
			return symbols[i].Confidence > symbols[j].Confidence
		}
		// Secondary: refCount DESC
		if symbols[i].RefCount != symbols[j].RefCount {
			return symbols[i].RefCount > symbols[j].RefCount
		}
		// Tertiary: stableId ASC
		return symbols[i].StableId < symbols[j].StableId
	})
}

// SortReferences sorts references by fileId ASC, startLine ASC, startColumn ASC
func SortReferences(refs []Reference) {
	sort.SliceStable(refs, func(i, j int) bool {
		// Primary: fileId ASC
		if refs[i].FileId != refs[j].FileId {
			return refs[i].FileId < refs[j].FileId
		}
		// Secondary: startLine ASC
		if refs[i].StartLine != refs[j].StartLine {
			return refs[i].StartLine < refs[j].StartLine
		}
		// Tertiary: startColumn ASC
		return refs[i].StartColumn < refs[j].StartColumn
	})
}

// SortImpactItems sorts impact items by kind priority, confidence DESC, stableId ASC
func SortImpactItems(items []ImpactItem) {
	sort.SliceStable(items, func(i, j int) bool {
		// Primary: kind priority
		iPriority := GetImpactKindPriority(items[i].Kind)
		jPriority := GetImpactKindPriority(items[j].Kind)
		if iPriority != jPriority {
			return iPriority < jPriority
		}
		// Secondary: confidence DESC
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		// Tertiary: stableId ASC
		return items[i].StableId < items[j].StableId
	})
}

// SortDrilldowns sorts drilldowns by relevanceScore DESC, label ASC
func SortDrilldowns(drilldowns []Drilldown) {
	sort.SliceStable(drilldowns, func(i, j int) bool {
		// Primary: relevanceScore DESC
		if drilldowns[i].RelevanceScore != drilldowns[j].RelevanceScore {
			return drilldowns[i].RelevanceScore > drilldowns[j].RelevanceScore
		}
		// Secondary: label ASC
		return drilldowns[i].Label < drilldowns[j].Label
	})
}

// SortWarnings sorts warnings by severity DESC, text ASC
func SortWarnings(warnings []Warning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		// Primary: severity DESC (by priority)
		iSev := GetWarningSeverity(warnings[i].Severity)
		jSev := GetWarningSeverity(warnings[j].Severity)
		if iSev != jSev {
			return iSev < jSev
		}
		// Secondary: text ASC
		return warnings[i].Text < warnings[j].Text
	})
}
