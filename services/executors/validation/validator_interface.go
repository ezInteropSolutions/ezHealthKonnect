package validation

import (
	"ezhealthkonnect/models"
)

// Validator defines the interface for all validators (Strategy Pattern)
type Validator interface {
	Validate(value interface{}, options map[string]interface{}) (bool, string)
	GetType() string
	GetDescription() string
}

// BaseValidator provides common functionality for all validators
type BaseValidator struct {
	validatorType string
	description   string
}

func (b *BaseValidator) GetType() string {
	return b.validatorType
}

func (b *BaseValidator) GetDescription() string {
	return b.description
}

// FieldValidationError helper
func NewFieldValidationError(field, fieldName, validationType, message, actualValue, expectedFormat string) models.FieldValidationError {
	return models.FieldValidationError{
		Field:          field,
		FieldName:      fieldName,
		Type:           validationType,
		Message:        message,
		ActualValue:    actualValue,
		ExpectedFormat: expectedFormat,
	}
}
