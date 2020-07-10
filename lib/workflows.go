package lib

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jbrunton/jflows/styles"
	"github.com/logrusorgru/aurora"

	"github.com/google/go-jsonnet"
	"github.com/spf13/afero"
)

// WorkflowDefinition - definitoin for a workflow defined by a JFlows template
type WorkflowDefinition struct {
	Name        string
	Source      string
	Destination string
	Content     string
	Status      ValidationResult
}

func getWorkflowSources(fs *afero.Afero, context *JFlowsContext) []string {
	files := []string{}
	err := fs.Walk(context.WorkflowsDir, func(path string, f os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if ext == ".jsonnet" || ext == ".libsonnet" {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	return files
}

func getWorkflowTemplates(fs *afero.Afero, context *JFlowsContext) []string {
	sources := getWorkflowSources(fs, context)
	var templates []string
	for _, source := range sources {
		if filepath.Ext(source) == ".jsonnet" {
			templates = append(templates, source)
		}
	}
	return templates
}

func getWorkflowName(workflowsDir string, filename string) string {
	_, templateFileName := filepath.Split(filename)
	return strings.TrimSuffix(templateFileName, filepath.Ext(templateFileName))
}

// GetWorkflowDefinitions - get workflow definitions for the given context
func GetWorkflowDefinitions(fs *afero.Afero, context *JFlowsContext) ([]*WorkflowDefinition, error) {
	vm := jsonnet.MakeVM()
	vm.StringOutput = true
	//vm.ErrorFormatter.SetColorFormatter(color.New(color.FgRed).Fprintf)

	templates := getWorkflowTemplates(fs, context)
	definitions := []*WorkflowDefinition{}
	for _, templatePath := range templates {
		workflowName := getWorkflowName(context.WorkflowsDir, templatePath)
		input, err := fs.ReadFile(templatePath)
		if err != nil {
			return []*WorkflowDefinition{}, err
		}

		destinationPath := filepath.Join(context.GitHubDir, "workflows/", workflowName+".yml")
		definition := &WorkflowDefinition{
			Name:        workflowName,
			Source:      templatePath,
			Destination: destinationPath,
			Status:      ValidationResult{Valid: true},
		}

		workflow, err := vm.EvaluateSnippet(templatePath, string(input))
		if err != nil {
			// fmt.Println(styles.StyleError(fmt.Sprintf("Error processing %s", templatePath)))
			// fmt.Println(err)
			// os.Exit(1)
			definition.Status.Valid = false
			definition.Status.Errors = []string{strings.Trim(err.Error(), " \n\r")}
		} else {
			meta := strings.Join([]string{
				"# File generated by jflows, do not modify",
				fmt.Sprintf("# Source: %s", templatePath),
			}, "\n")
			definition.Content = meta + "\n" + workflow
		}
		definitions = append(definitions, definition)
	}

	return definitions, nil
}

// UpdateWorkflows - update workflow files for the given context
func UpdateWorkflows(fs *afero.Afero, context *JFlowsContext) {
	definitions, err := GetWorkflowDefinitions(fs, context)
	if err != nil {
		panic(err) // TODO: improve this
	}
	for _, definition := range definitions {
		if definition.Status.Valid {
			updateFileContent(fs, definition.Destination, definition.Content, fmt.Sprintf("(from %s)", definition.Source))
		} else {
			// TODO: warning
		}
	}
}

// ValidateWorkflows - returns an error if the workflows are out of date
func ValidateWorkflows(fs *afero.Afero, context *JFlowsContext) error {
	WorkflowValidator := NewWorkflowValidator(fs)
	definitions, err := GetWorkflowDefinitions(fs, context)
	if err != nil {
		return err
	}
	valid := true
	for _, definition := range definitions {
		fmt.Printf("Checking %s ... ", aurora.Bold(definition.Name))

		if !definition.Status.Valid {
			fmt.Println(styles.StyleError("FAILED"))
			fmt.Println("  Error parsing template:")
			for _, err := range definition.Status.Errors {
				fmt.Printf("  ► %s\n\n", err)
			}
			valid = false
			continue
		}

		schemaResult := WorkflowValidator.ValidateSchema(definition)
		if !schemaResult.Valid {
			fmt.Println(styles.StyleError("FAILED"))
			fmt.Println("  Workflow failed schema validation:")
			for _, err := range schemaResult.Errors {
				fmt.Printf("  ► %s\n", err)
			}
			valid = false
			continue
		}

		contentResult := WorkflowValidator.ValidateContent(definition)
		if !contentResult.Valid {
			fmt.Println(styles.StyleError("FAILED"))
			fmt.Println("  " + contentResult.Errors[0])
			fmt.Println("  ► Run \"jflows workflow update\" to update")
			valid = false
			continue
		}

		fmt.Println(styles.StyleOK("OK"))
	}
	if !valid {
		return errors.New("workflow validation failed")
	}
	return nil
}
