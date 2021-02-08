package engine

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrunton/gflows/env"

	"github.com/jbrunton/gflows/config"
	"github.com/jbrunton/gflows/io/content"
	"github.com/jbrunton/gflows/io/pkg"
	"github.com/jbrunton/gflows/workflow"
	"github.com/jbrunton/gflows/yamlutil"

	"github.com/jbrunton/gflows/fixtures"
	"github.com/stretchr/testify/assert"
)

func newJsonnetTemplateEngine(config string, roundTripper http.RoundTripper) (*content.Container, *config.GFlowsContext, *JsonnetTemplateEngine) {
	if config == "" {
		config = "templates:\n  engine: jsonnet"
	}
	ioContainer, context, _ := fixtures.NewTestContext(config)
	container := content.NewContainer(ioContainer, &http.Client{Transport: roundTripper})
	installer := env.NewGFlowsLibInstaller(container.FileSystem(), container.ContentReader(), container.ContentWriter(), container.Logger())
	env := env.NewGFlowsEnv(container.FileSystem(), installer, context, container.Logger())
	templateEngine := NewJsonnetTemplateEngine(container.FileSystem(), context, container.ContentWriter(), env)
	return container, context, templateEngine
}

func TestGetJsonnetWorkflowDefinitions(t *testing.T) {
	container, _, templateEngine := newJsonnetTemplateEngine("", fixtures.NewMockRoundTripper())
	fs := container.FileSystem()
	fs.WriteFile(".gflows/workflows/test.jsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)

	definitions, _ := templateEngine.GetWorkflowDefinitions()

	expectedContent := fixtures.ExampleWorkflow("test.jsonnet")
	expectedJson, _ := yamlutil.YamlToJson(expectedContent)
	expectedDefinition := workflow.Definition{
		Name:        "test",
		Source:      ".gflows/workflows/test.jsonnet",
		Description: ".gflows/workflows/test.jsonnet",
		Destination: ".github/workflows/test.yml",
		Content:     expectedContent,
		Status:      workflow.ValidationResult{Valid: true},
		JSON:        expectedJson,
	}
	assert.Equal(t, []*workflow.Definition{&expectedDefinition}, definitions)
}

func TestGetJsonnetWorkflowDefinitionsWithLibs(t *testing.T) {
	config := strings.Join([]string{
		"templates:",
		"  engine: jsonnet",
		"  defaults:",
		"    dependencies:",
		"    - /path/to/my-lib",
	}, "\n")
	container, _, templateEngine := newJsonnetTemplateEngine(config, fixtures.NewMockRoundTripper())
	fs := container.FileSystem()
	fs.WriteFile(".gflows/workflows/test.jsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	container.ContentWriter().SafelyWriteFile("/path/to/my-lib/gflowspkg.json", `{"files": ["workflows/lib-workflow.jsonnet"]}`)
	container.ContentWriter().SafelyWriteFile("/path/to/my-lib/workflows/lib-workflow.jsonnet", `std.manifestYamlDoc({})`)
	lib, _ := templateEngine.env.LoadDependency("/path/to/my-lib")

	definitions, _ := templateEngine.GetWorkflowDefinitions()

	expectedLocalContent := fixtures.ExampleWorkflow("test.jsonnet")
	expectedLocalJson, _ := yamlutil.YamlToJson(expectedLocalContent)
	expectedLocalDefinition := workflow.Definition{
		Name:        "test",
		Source:      ".gflows/workflows/test.jsonnet",
		Description: ".gflows/workflows/test.jsonnet",
		Destination: ".github/workflows/test.yml",
		Content:     expectedLocalContent,
		Status:      workflow.ValidationResult{Valid: true},
		JSON:        expectedLocalJson,
	}
	expectedRemoteDefinition := workflow.Definition{
		Name:        "lib-workflow",
		Source:      filepath.Join(lib.LocalDir, "workflows/lib-workflow.jsonnet"),
		Description: "my-lib/workflows/lib-workflow.jsonnet",
		Destination: ".github/workflows/lib-workflow.yml",
		Content:     "# File generated by gflows, do not modify\n# Source: my-lib/workflows/lib-workflow.jsonnet\n{}\n",
		Status:      workflow.ValidationResult{Valid: true},
		JSON:        make(map[string]interface{}),
	}
	assert.Equal(t, []*workflow.Definition{&expectedRemoteDefinition, &expectedLocalDefinition}, definitions)
}

func TestSerializationError(t *testing.T) {
	container, _, templateEngine := newJsonnetTemplateEngine("", fixtures.NewMockRoundTripper())
	fs := container.FileSystem()
	fs.WriteFile(".gflows/workflows/test.jsonnet", []byte("{}"), 0644)

	definitions, _ := templateEngine.GetWorkflowDefinitions()

	expectedError := strings.Join([]string{
		"RUNTIME ERROR: expected string result, got: object",
		"\tDuring manifestation\t",
		"You probably need to serialize the output to YAML. See https://github.com/jbrunton/gflows/wiki/Templates#serialization",
	}, "\n")
	expectedDefinition := workflow.Definition{
		Name:        "test",
		Source:      ".gflows/workflows/test.jsonnet",
		Destination: ".github/workflows/test.yml",
		Content:     "",
		Status: workflow.ValidationResult{
			Valid:  false,
			Errors: []string{expectedError},
		},
		JSON: nil,
	}
	assert.Equal(t, []*workflow.Definition{&expectedDefinition}, definitions)
}

func TestGetJsonnetObservableSources(t *testing.T) {
	config := strings.Join([]string{
		"templates:",
		"  engine: jsonnet",
		"  defaults:",
		"    libs:",
		"    - vendor",
		"    - foo/bar.libsonnet",
		"    - https://example.com/config.yml",
	}, "\n")
	container, _, templateEngine := newJsonnetTemplateEngine(config, fixtures.NewMockRoundTripper())
	fs := container.FileSystem()
	fs.WriteFile(".gflows/workflows/test.jsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile(".gflows/workflows/test.libsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile(".gflows/workflows/invalid.ext", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile(".gflows/libs/lib.libsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile("vendor/lib.libsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile("foo/bar.libsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)

	sources, err := templateEngine.GetObservableSources()

	assert.NoError(t, err)
	assert.Equal(t, []string{
		"vendor/lib.libsonnet",
		"foo/bar.libsonnet",
		".gflows/workflows/test.jsonnet",
		".gflows/workflows/test.libsonnet",
		".gflows/libs/lib.libsonnet",
	}, sources)
}

func TestGetJsonnetWorkflowTemplates(t *testing.T) {
	container, _, templateEngine := newJsonnetTemplateEngine("", fixtures.NewMockRoundTripper())
	fs := container.FileSystem()
	fs.WriteFile(".gflows/workflows/test.jsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile(".gflows/workflows/test.libsonnet", []byte(fixtures.ExampleJsonnetTemplate), 0644)
	fs.WriteFile(".gflows/workflows/invalid.ext", []byte(fixtures.ExampleJsonnetTemplate), 0644)

	templates, err := templateEngine.getWorkflowTemplates()

	expectedPaths := []*pkg.PathInfo{
		&pkg.PathInfo{
			SourcePath:  ".gflows/workflows/test.jsonnet",
			LocalPath:   ".gflows/workflows/test.jsonnet",
			Description: ".gflows/workflows/test.jsonnet",
		},
	}
	assert.NoError(t, err)
	assert.Equal(t, expectedPaths, templates)
}

func TestGetJsonnetWorkflowName(t *testing.T) {
	_, _, templateEngine := newJsonnetTemplateEngine("", fixtures.NewMockRoundTripper())
	assert.Equal(t, "my-workflow-1", templateEngine.getWorkflowName("/workflows/my-workflow-1.jsonnet"))
	assert.Equal(t, "my-workflow-2", templateEngine.getWorkflowName("/workflows/workflows/my-workflow-2.jsonnet"))
}
