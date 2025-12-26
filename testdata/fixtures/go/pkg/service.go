// Package pkg provides HTTP handling and business logic.
package pkg

import "fixture/internal"

// Service defines the business logic interface.
type Service interface {
	Process(input string) string
	Validate(input string) error
}

// DefaultService is the default implementation of Service.
type DefaultService struct {
	model *Model
}

// NewDefaultService creates a new DefaultService.
func NewDefaultService() *DefaultService {
	return &DefaultService{
		model: NewModel("default"),
	}
}

// Process implements Service.Process.
// Call chain: Process -> model.Transform -> internal.FormatOutput
func (s *DefaultService) Process(input string) string {
	transformed := s.model.Transform(input)
	return internal.FormatOutput(transformed)
}

// Validate implements Service.Validate.
func (s *DefaultService) Validate(input string) error {
	if input == "" {
		return internal.ErrEmptyInput
	}
	return nil
}

// CachingService wraps a Service with caching.
type CachingService struct {
	inner Service
	cache map[string]string
}

// NewCachingService creates a caching wrapper around a Service.
func NewCachingService(inner Service) *CachingService {
	return &CachingService{
		inner: inner,
		cache: make(map[string]string),
	}
}

// Process implements Service.Process with caching.
func (c *CachingService) Process(input string) string {
	if cached, ok := c.cache[input]; ok {
		return cached
	}
	result := c.inner.Process(input)
	c.cache[input] = result
	return result
}

// Validate implements Service.Validate by delegating.
func (c *CachingService) Validate(input string) error {
	return c.inner.Validate(input)
}
