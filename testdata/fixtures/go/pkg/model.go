// Package pkg provides HTTP handling and business logic.
package pkg

import "strings"

// Model represents the core data model.
type Model struct {
	Name   string
	Config *Config
}

// Config holds model configuration.
type Config struct {
	Prefix    string
	Uppercase bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Prefix:    ">>",
		Uppercase: false,
	}
}

// NewModel creates a new Model with the given name.
func NewModel(name string) *Model {
	return &Model{
		Name:   name,
		Config: DefaultConfig(),
	}
}

// Transform applies the model transformation to input.
func (m *Model) Transform(input string) string {
	result := m.Config.Prefix + " " + input
	if m.Config.Uppercase {
		result = strings.ToUpper(result)
	}
	return result
}

// SetConfig updates the model configuration.
func (m *Model) SetConfig(cfg *Config) {
	m.Config = cfg
}

// Clone creates a copy of the model.
func (m *Model) Clone() *Model {
	return &Model{
		Name: m.Name,
		Config: &Config{
			Prefix:    m.Config.Prefix,
			Uppercase: m.Config.Uppercase,
		},
	}
}
