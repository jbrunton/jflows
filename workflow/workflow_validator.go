package workflow

import (
	"fmt"

	"github.com/jbrunton/gflows/config"
	"github.com/spf13/afero"
	"github.com/xeipuuv/gojsonschema"
)

// WorkflowValidator - validates a workflow definition
type WorkflowValidator struct {
	fs            *afero.Afero
	defaultSchema *gojsonschema.Schema
	config        *config.GFlowsConfig
}

// ValidationResult - validate result
type ValidationResult struct {
	Valid         bool
	Errors        []string
	ActualContent string
}

// NewWorkflowValidator - creates a new validator for the given filesystem
func NewWorkflowValidator(fs *afero.Afero, context *config.GFlowsContext) *WorkflowValidator {
	config := context.Config
	schemaLoader := gojsonschema.NewReferenceLoader(config.Workflows.Defaults.Checks.Schema.URI)
	defaultSchema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		panic(err)
	}
	return &WorkflowValidator{
		fs:            fs,
		defaultSchema: defaultSchema,
		config:        config,
	}
}

// ValidateSchema - validates the template for the definition generates a valid workflow
func (validator *WorkflowValidator) ValidateSchema(definition *Definition) ValidationResult {
	enabled := validator.getSchemaCheckEnabled(definition)
	if !enabled {
		return ValidationResult{
			Valid:  true,
			Errors: []string{fmt.Sprintf("Schema checks disabled for %s, skipping", definition.Name)},
		}
	}

	loader := gojsonschema.NewGoLoader(definition.JSON)
	schema := validator.getWorkflowSchema(definition.Name)
	result, err := schema.Validate(loader)
	if err != nil {
		panic(err)
	}

	errors := []string{}
	for _, error := range result.Errors() {
		errors = append(errors, error.String())
	}

	return ValidationResult{
		Valid:  result.Valid(),
		Errors: errors,
	}
}

// ValidateContent - validates the content at the destination in the definition is up to date
func (validator *WorkflowValidator) ValidateContent(definition *Definition) ValidationResult {
	enabled := validator.getContentCheckEnabled(definition)
	if !enabled {
		return ValidationResult{
			Valid:  true,
			Errors: []string{fmt.Sprintf("Content checks disabled for %s, skipping", definition.Name)},
		}
	}

	exists, err := validator.fs.Exists(definition.Destination)
	if err != nil {
		panic(err)
	}

	if !exists {
		reason := fmt.Sprintf("Workflow missing for %q (expected workflow at %s)", definition.Name, definition.Destination)
		return ValidationResult{
			Valid:         false,
			Errors:        []string{reason},
			ActualContent: "",
		}
	}

	data, err := validator.fs.ReadFile(definition.Destination)
	if err != nil {
		panic(err)
	}

	actualContent := string(data)
	if actualContent != definition.Content {
		reason := fmt.Sprintf("Content is out of date for %q (%s)", definition.Name, definition.Destination)
		return ValidationResult{
			Valid:         false,
			Errors:        []string{reason},
			ActualContent: actualContent,
		}
	}

	return ValidationResult{
		Valid:  true,
		Errors: []string{},
	}
}

func (validator *WorkflowValidator) getWorkflowSchema(workflowName string) *gojsonschema.Schema {
	workflowConfig := validator.config.Workflows.Overrides[workflowName]
	if workflowConfig == nil || workflowConfig.Checks.Schema.URI == "" {
		return validator.defaultSchema
	}
	schemaLoader := gojsonschema.NewReferenceLoader(workflowConfig.Checks.Schema.URI)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		panic(err)
	}
	return schema
}

func (validator *WorkflowValidator) getContentCheckEnabled(definition *Definition) bool {
	return validator.config.GetWorkflowBoolProperty(definition.Name, true, func(config *config.GFlowsWorkflowConfig) *bool {
		return config.Checks.Content.Enabled
	})
}

func (validator *WorkflowValidator) getSchemaCheckEnabled(definition *Definition) bool {
	return validator.config.GetWorkflowBoolProperty(definition.Name, true, func(config *config.GFlowsWorkflowConfig) *bool {
		return config.Checks.Schema.Enabled
	})
}
