package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var methodOrder = map[string]int{
	"get":     0,
	"post":    1,
	"put":     2,
	"patch":   3,
	"delete":  4,
	"options": 5,
	"head":    6,
}

type document struct {
	OpenAPI    string                `yaml:"openapi"`
	Info       info                  `yaml:"info"`
	Servers    []server              `yaml:"servers"`
	Tags       []tag                 `yaml:"tags"`
	Security   []securityRequirement `yaml:"security"`
	Paths      map[string]pathItem   `yaml:"paths"`
	Components components            `yaml:"components"`
}

type info struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type server struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

type tag struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type components struct {
	SecuritySchemes map[string]securityScheme `yaml:"securitySchemes"`
	Schemas         map[string]schemaSummary  `yaml:"schemas"`
}

type securityScheme struct {
	Type         string `yaml:"type"`
	In           string `yaml:"in"`
	Name         string `yaml:"name"`
	Scheme       string `yaml:"scheme"`
	BearerFormat string `yaml:"bearerFormat"`
	Description  string `yaml:"description"`
}

type securityRequirement map[string][]string

type pathItem struct {
	Get     *operation `yaml:"get"`
	Post    *operation `yaml:"post"`
	Put     *operation `yaml:"put"`
	Patch   *operation `yaml:"patch"`
	Delete  *operation `yaml:"delete"`
	Options *operation `yaml:"options"`
	Head    *operation `yaml:"head"`
}

type operation struct {
	Summary     string                `yaml:"summary"`
	Description string                `yaml:"description"`
	OperationID string                `yaml:"operationId"`
	Tags        []string              `yaml:"tags"`
	Parameters  []parameter           `yaml:"parameters"`
	RequestBody *requestBody          `yaml:"requestBody"`
	Responses   map[string]response   `yaml:"responses"`
	Security    []securityRequirement `yaml:"security"`
}

type parameter struct {
	Name        string    `yaml:"name"`
	In          string    `yaml:"in"`
	Description string    `yaml:"description"`
	Required    bool      `yaml:"required"`
	Schema      schemaRef `yaml:"schema"`
}

type requestBody struct {
	Description string               `yaml:"description"`
	Required    bool                 `yaml:"required"`
	Content     map[string]mediaType `yaml:"content"`
}

type response struct {
	Description string               `yaml:"description"`
	Content     map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema schemaRef `yaml:"schema"`
}

type schemaSummary struct {
	Type                 string               `yaml:"type"`
	Description          string               `yaml:"description"`
	Properties           map[string]schemaRef `yaml:"properties"`
	Required             []string             `yaml:"required"`
	Enum                 []string             `yaml:"enum"`
	Items                *schemaRef           `yaml:"items"`
	OneOf                []schemaRef          `yaml:"oneOf"`
	AnyOf                []schemaRef          `yaml:"anyOf"`
	AllOf                []schemaRef          `yaml:"allOf"`
	AdditionalProperties any                  `yaml:"additionalProperties"`
}

type schemaRef struct {
	Ref                  string      `yaml:"$ref"`
	Type                 string      `yaml:"type"`
	Format               string      `yaml:"format"`
	Description          string      `yaml:"description"`
	Enum                 []string    `yaml:"enum"`
	Items                *schemaRef  `yaml:"items"`
	OneOf                []schemaRef `yaml:"oneOf"`
	AnyOf                []schemaRef `yaml:"anyOf"`
	AllOf                []schemaRef `yaml:"allOf"`
	AdditionalProperties any         `yaml:"additionalProperties"`
}

type operationEntry struct {
	Tag    string
	Path   string
	Method string
	Body   operation
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("generate-openapi-docs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	inputPathFlag := flags.String("input", "openapi.yaml", "Path to the OpenAPI source document.")
	outputPathFlag := flags.String("output", "docs/generated/openapi-reference.md", "Path to the generated Markdown reference.")
	checkMode := flags.Bool("check", false, "Fail if the generated Markdown would change the output file.")

	if err := flags.Parse(args); err != nil {
		return err
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	inputPath := resolvePath(repoRoot, *inputPathFlag)
	outputPath := resolvePath(repoRoot, *outputPathFlag)

	doc, err := loadDocument(inputPath)
	if err != nil {
		return err
	}

	rendered := renderMarkdown(doc, filepath.Base(inputPath))

	if *checkMode {
		existing, errRead := os.ReadFile(outputPath)
		if errRead != nil {
			if errors.Is(errRead, os.ErrNotExist) {
				return fmt.Errorf("%s does not exist; run `go run ./cmd/generate-openapi-docs`", outputPath)
			}
			return fmt.Errorf("read %s: %w", outputPath, errRead)
		}
		if !bytes.Equal(existing, rendered) {
			return fmt.Errorf("%s is out of date; run `go run ./cmd/generate-openapi-docs`", outputPath)
		}
		fmt.Printf("up to date: %s\n", outputPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory for %s: %w", outputPath, err)
	}
	if err := os.WriteFile(outputPath, rendered, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}

	fmt.Printf("wrote %s\n", outputPath)
	return nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	current := wd
	for {
		if fileExists(filepath.Join(current, "go.mod")) && fileExists(filepath.Join(current, "openapi.yaml")) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("failed to locate repository root containing go.mod and openapi.yaml")
		}
		current = parent
	}
}

func resolvePath(repoRoot, pathValue string) string {
	if filepath.IsAbs(pathValue) {
		return filepath.Clean(pathValue)
	}
	return filepath.Join(repoRoot, filepath.FromSlash(pathValue))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadDocument(path string) (document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return document{}, fmt.Errorf("read %s: %w", path, err)
	}

	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return document{}, fmt.Errorf("parse %s: %w", path, err)
	}

	if len(doc.Paths) == 0 {
		return document{}, fmt.Errorf("%s does not define any paths", path)
	}
	if len(doc.Components.Schemas) == 0 {
		return document{}, fmt.Errorf("%s does not define any component schemas", path)
	}

	return doc, nil
}

func renderMarkdown(doc document, sourceName string) []byte {
	var buf bytes.Buffer

	writeLine(&buf, "# CLIProxyAPI HTTP API Reference")
	writeLine(&buf, "")
	writeLine(&buf, fmt.Sprintf("> This file is auto-generated from `%s` by `go run ./cmd/generate-openapi-docs`. Do not edit it manually.", sourceName))
	writeLine(&buf, "")

	writeLine(&buf, "## Overview")
	writeLine(&buf, "")
	writeLine(&buf, fmt.Sprintf("- **Title:** %s", markdownInline(doc.Info.Title)))
	writeLine(&buf, fmt.Sprintf("- **Version:** `%s`", strings.TrimSpace(doc.Info.Version)))
	writeLine(&buf, fmt.Sprintf("- **OpenAPI:** `%s`", strings.TrimSpace(doc.OpenAPI)))
	writeLine(&buf, fmt.Sprintf("- **Tags:** %d", len(doc.Tags)))
	writeLine(&buf, fmt.Sprintf("- **Paths:** %d", len(doc.Paths)))
	writeLine(&buf, fmt.Sprintf("- **Component Schemas:** %d", len(doc.Components.Schemas)))
	if description := singleLine(doc.Info.Description); description != "" {
		writeLine(&buf, "")
		writeLine(&buf, description)
	}
	writeLine(&buf, "")

	writeLine(&buf, "## Servers")
	writeLine(&buf, "")
	writeLine(&buf, "| URL | Description |")
	writeLine(&buf, "| --- | --- |")
	for _, item := range doc.Servers {
		writeLine(&buf, fmt.Sprintf("| `%s` | %s |", item.URL, markdownCell(item.Description)))
	}
	writeLine(&buf, "")

	writeLine(&buf, "## Authentication")
	writeLine(&buf, "")
	writeLine(&buf, fmt.Sprintf("Default security requirement: %s", markdownInline(securitySummary(doc.Security))))
	writeLine(&buf, "")
	writeLine(&buf, "| Scheme | Type | Description |")
	writeLine(&buf, "| --- | --- | --- |")
	for _, name := range sortedKeys(doc.Components.SecuritySchemes) {
		scheme := doc.Components.SecuritySchemes[name]
		writeLine(&buf, fmt.Sprintf("| `%s` | %s | %s |", name, markdownCell(securitySchemeType(scheme)), markdownCell(scheme.Description)))
	}
	writeLine(&buf, "")

	operationsByTag := collectOperations(doc)
	for _, tagName := range orderedTagNames(doc.Tags, operationsByTag) {
		entries := operationsByTag[tagName]
		tagDescription := tagDescription(doc.Tags, tagName)

		writeLine(&buf, fmt.Sprintf("## %s Endpoints", tagName))
		writeLine(&buf, "")
		if tagDescription != "" {
			writeLine(&buf, tagDescription)
			writeLine(&buf, "")
		}

		writeLine(&buf, "| Method | Path | Summary | Request Body | Responses |")
		writeLine(&buf, "| --- | --- | --- | --- | --- |")
		for _, entry := range entries {
			writeLine(&buf, fmt.Sprintf(
				"| %s | `%s` | %s | %s | %s |",
				strings.ToUpper(entry.Method),
				entry.Path,
				markdownCell(entry.Body.Summary),
				markdownCell(requestBodySummary(entry.Body.RequestBody)),
				markdownCell(responseSummary(entry.Body.Responses)),
			))
		}
		writeLine(&buf, "")

		for _, entry := range entries {
			renderOperationDetails(&buf, entry, doc.Security)
		}
	}

	writeLine(&buf, "## Component Schemas")
	writeLine(&buf, "")
	writeLine(&buf, "| Schema | Type | Properties | Required | Description |")
	writeLine(&buf, "| --- | --- | ---: | ---: | --- |")
	for _, name := range sortedKeys(doc.Components.Schemas) {
		schema := doc.Components.Schemas[name]
		writeLine(&buf, fmt.Sprintf(
			"| `%s` | %s | %d | %d | %s |",
			name,
			markdownCell(schemaSummaryLabel(schema)),
			len(schema.Properties),
			len(schema.Required),
			markdownCell(schema.Description),
		))
	}

	return buf.Bytes()
}

func renderOperationDetails(buf *bytes.Buffer, entry operationEntry, globalSecurity []securityRequirement) {
	writeLine(buf, fmt.Sprintf("### %s `%s`", strings.ToUpper(entry.Method), entry.Path))
	writeLine(buf, "")

	if summary := singleLine(entry.Body.Summary); summary != "" {
		writeLine(buf, summary)
		writeLine(buf, "")
	}
	if description := singleLine(entry.Body.Description); description != "" {
		writeLine(buf, description)
		writeLine(buf, "")
	}

	writeLine(buf, fmt.Sprintf("- **Operation ID:** %s", markdownInline(orFallback(entry.Body.OperationID, "Not declared"))))
	writeLine(buf, fmt.Sprintf("- **Security:** %s", markdownInline(securitySummary(effectiveSecurity(entry.Body.Security, globalSecurity)))))
	writeLine(buf, fmt.Sprintf("- **Tags:** %s", markdownInline(tagList(entry.Body.Tags, entry.Tag))))
	writeLine(buf, "")

	writeLine(buf, "#### Parameters")
	writeLine(buf, "")
	if len(entry.Body.Parameters) == 0 {
		writeLine(buf, "None.")
		writeLine(buf, "")
	} else {
		writeLine(buf, "| Name | In | Required | Schema | Description |")
		writeLine(buf, "| --- | --- | --- | --- | --- |")
		for _, parameter := range sortParameters(entry.Body.Parameters) {
			writeLine(buf, fmt.Sprintf(
				"| `%s` | `%s` | %s | %s | %s |",
				parameter.Name,
				parameter.In,
				boolLabel(parameter.Required),
				markdownCell(schemaRefLabel(parameter.Schema)),
				markdownCell(parameter.Description),
			))
		}
		writeLine(buf, "")
	}

	writeLine(buf, "#### Request Body")
	writeLine(buf, "")
	if entry.Body.RequestBody == nil {
		writeLine(buf, "None.")
		writeLine(buf, "")
	} else {
		writeLine(buf, fmt.Sprintf("- Required: %s", boolLabel(entry.Body.RequestBody.Required)))
		if description := singleLine(entry.Body.RequestBody.Description); description != "" {
			writeLine(buf, fmt.Sprintf("- Description: %s", description))
		}
		writeLine(buf, "")
		writeLine(buf, "| Content Type | Schema |")
		writeLine(buf, "| --- | --- |")
		for _, contentType := range sortedKeys(entry.Body.RequestBody.Content) {
			writeLine(buf, fmt.Sprintf("| `%s` | %s |", contentType, markdownCell(schemaRefLabel(entry.Body.RequestBody.Content[contentType].Schema))))
		}
		writeLine(buf, "")
	}

	writeLine(buf, "#### Responses")
	writeLine(buf, "")
	writeLine(buf, "| Status | Description | Schema |")
	writeLine(buf, "| --- | --- | --- |")
	for _, statusCode := range sortedStatusCodes(entry.Body.Responses) {
		item := entry.Body.Responses[statusCode]
		writeLine(buf, fmt.Sprintf(
			"| `%s` | %s | %s |",
			statusCode,
			markdownCell(item.Description),
			markdownCell(contentSummary(item.Content)),
		))
	}
	writeLine(buf, "")
}

func collectOperations(doc document) map[string][]operationEntry {
	operationsByTag := make(map[string][]operationEntry)

	for path, item := range doc.Paths {
		for method, body := range item.operations() {
			if body == nil {
				continue
			}

			tags := body.Tags
			if len(tags) == 0 {
				tags = []string{"Untagged"}
			}
			for _, tagName := range tags {
				operationsByTag[tagName] = append(operationsByTag[tagName], operationEntry{
					Tag:    tagName,
					Path:   path,
					Method: method,
					Body:   *body,
				})
			}
		}
	}

	for tagName := range operationsByTag {
		sort.Slice(operationsByTag[tagName], func(i, j int) bool {
			left := operationsByTag[tagName][i]
			right := operationsByTag[tagName][j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			return methodOrder[left.Method] < methodOrder[right.Method]
		})
	}

	return operationsByTag
}

func (item pathItem) operations() map[string]*operation {
	return map[string]*operation{
		"get":     item.Get,
		"post":    item.Post,
		"put":     item.Put,
		"patch":   item.Patch,
		"delete":  item.Delete,
		"options": item.Options,
		"head":    item.Head,
	}
}

func orderedTagNames(tags []tag, operationsByTag map[string][]operationEntry) []string {
	ordered := make([]string, 0, len(operationsByTag))
	seen := make(map[string]struct{}, len(operationsByTag))

	for _, item := range tags {
		if _, ok := operationsByTag[item.Name]; !ok {
			continue
		}
		ordered = append(ordered, item.Name)
		seen[item.Name] = struct{}{}
	}

	var extras []string
	for tagName := range operationsByTag {
		if _, ok := seen[tagName]; ok {
			continue
		}
		extras = append(extras, tagName)
	}
	sort.Strings(extras)

	return append(ordered, extras...)
}

func tagDescription(tags []tag, name string) string {
	for _, item := range tags {
		if item.Name == name {
			return singleLine(item.Description)
		}
	}
	return ""
}

func securitySchemeType(scheme securityScheme) string {
	var details []string
	if scheme.Type != "" {
		details = append(details, scheme.Type)
	}
	if scheme.In != "" && scheme.Name != "" {
		details = append(details, fmt.Sprintf("%s %s", scheme.In, scheme.Name))
	}
	if scheme.Scheme != "" {
		details = append(details, scheme.Scheme)
	}
	if scheme.BearerFormat != "" {
		details = append(details, scheme.BearerFormat)
	}
	if len(details) == 0 {
		return "unspecified"
	}
	return strings.Join(details, " / ")
}

func securitySummary(requirements []securityRequirement) string {
	if len(requirements) == 0 {
		return "None"
	}

	parts := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		if len(requirement) == 0 {
			parts = append(parts, "None")
			continue
		}

		keys := sortedRequirementKeys(requirement)
		parts = append(parts, strings.Join(keys, " + "))
	}

	return strings.Join(parts, " or ")
}

func sortedRequirementKeys(requirement securityRequirement) []string {
	keys := make([]string, 0, len(requirement))
	for key := range requirement {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func effectiveSecurity(operationSecurity, globalSecurity []securityRequirement) []securityRequirement {
	if operationSecurity != nil {
		return operationSecurity
	}
	return globalSecurity
}

func requestBodySummary(body *requestBody) string {
	if body == nil {
		return "None"
	}
	prefix := "Optional"
	if body.Required {
		prefix = "Required"
	}
	return prefix + ": " + contentSummary(body.Content)
}

func responseSummary(responses map[string]response) string {
	var parts []string
	for _, statusCode := range sortedStatusCodes(responses) {
		parts = append(parts, fmt.Sprintf("%s → %s", statusCode, contentSummary(responses[statusCode].Content)))
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, "; ")
}

func contentSummary(content map[string]mediaType) string {
	if len(content) == 0 {
		return "—"
	}

	parts := make([]string, 0, len(content))
	for _, contentType := range sortedKeys(content) {
		parts = append(parts, fmt.Sprintf("%s → %s", contentType, schemaRefLabel(content[contentType].Schema)))
	}
	return strings.Join(parts, "; ")
}

func schemaRefLabel(schema schemaRef) string {
	switch {
	case schema.Ref != "":
		return schemaRefName(schema.Ref)
	case len(schema.OneOf) > 0:
		return joinSchemaLabels(schema.OneOf, " | ")
	case len(schema.AnyOf) > 0:
		return joinSchemaLabels(schema.AnyOf, " or ")
	case len(schema.AllOf) > 0:
		return joinSchemaLabels(schema.AllOf, " + ")
	case schema.Type == "array" && schema.Items != nil:
		return "array<" + schemaRefLabel(*schema.Items) + ">"
	case schema.Type != "":
		if schema.Format != "" {
			return schema.Type + " (" + schema.Format + ")"
		}
		if len(schema.Enum) > 0 {
			return schema.Type + " enum"
		}
		return schema.Type
	case schema.AdditionalProperties != nil:
		return "object"
	default:
		return "unspecified"
	}
}

func schemaSummaryLabel(schema schemaSummary) string {
	switch {
	case len(schema.OneOf) > 0:
		return joinSchemaLabels(schema.OneOf, " | ")
	case len(schema.AnyOf) > 0:
		return joinSchemaLabels(schema.AnyOf, " or ")
	case len(schema.AllOf) > 0:
		return joinSchemaLabels(schema.AllOf, " + ")
	case schema.Type == "array" && schema.Items != nil:
		return "array<" + schemaRefLabel(*schema.Items) + ">"
	case schema.Type != "":
		if len(schema.Enum) > 0 {
			return schema.Type + " enum"
		}
		return schema.Type
	case len(schema.Properties) > 0 || schema.AdditionalProperties != nil:
		return "object"
	default:
		return "unspecified"
	}
}

func joinSchemaLabels(items []schemaRef, separator string) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, schemaRefLabel(item))
	}
	return strings.Join(parts, separator)
}

func schemaRefName(ref string) string {
	const prefix = "#/components/schemas/"
	if trimmed, ok := strings.CutPrefix(ref, prefix); ok {
		return trimmed
	}
	return ref
}

func sortedStatusCodes(responses map[string]response) []string {
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}

	sort.Slice(codes, func(i, j int) bool {
		leftValue, leftNumeric := statusCodeSortValue(codes[i])
		rightValue, rightNumeric := statusCodeSortValue(codes[j])
		if leftNumeric != rightNumeric {
			return leftNumeric
		}
		if leftValue != rightValue {
			return leftValue < rightValue
		}
		return codes[i] < codes[j]
	})

	return codes
}

func statusCodeSortValue(code string) (int, bool) {
	value, err := strconv.Atoi(code)
	if err == nil {
		return value, true
	}
	return 0, false
}

func sortParameters(parameters []parameter) []parameter {
	sorted := append([]parameter(nil), parameters...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].In != sorted[j].In {
			return sorted[i].In < sorted[j].In
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

func tagList(operationTags []string, fallback string) string {
	if len(operationTags) == 0 {
		return fallback
	}
	return strings.Join(operationTags, ", ")
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func singleLine(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func markdownCell(value string) string {
	clean := singleLine(value)
	if clean == "" {
		return "—"
	}
	clean = strings.ReplaceAll(clean, "|", "\\|")
	return clean
}

func markdownInline(value string) string {
	clean := markdownCell(value)
	if clean == "—" {
		return clean
	}
	return "`" + strings.ReplaceAll(clean, "`", "'") + "`"
}

func writeLine(buf *bytes.Buffer, line string) {
	buf.WriteString(line)
	buf.WriteByte('\n')
}

func boolLabel(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func orFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
