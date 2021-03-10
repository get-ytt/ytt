// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package template_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/k14s/difflib"
	cmdtpl "github.com/k14s/ytt/pkg/cmd/template"
	"github.com/k14s/ytt/pkg/cmd/ui"
	"github.com/k14s/ytt/pkg/files"
)

func TestDataValueConformingToSchemaSucceeds(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("map only", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
db_conn:
  hostname: ""
  port: 0
  username: ""
  password: ""
  metadata:
    run: jobName
  tls_only: false
top_level: ""
`
		dataValuesYAML := `#@data/values
---
db_conn:
  hostname: server.example.com
  port: 5432
  username: sa
  password: changeme
  metadata:
    run: ./build.sh
  tls_only: true
top_level: key
`
		templateYAML := `---
rendered: true`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, "rendered: true\n", opts)
	})
	t.Run("map and array", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
db_conn:
- hostname: ""
  port: 0
  username: ""
  password: ""
  metadata:
    run: jobName
top_level: ""
`
		dataValuesYAML := `#@data/values
---
db_conn:
- hostname: server.example.com
  port: 5432
  username: sa
  password: changeme
  metadata:
    run: ./build.sh
top_level: key
`
		templateYAML := `#@ load("@ytt:data", "data")
---
rendered: #@ data.values
`

		expected := `rendered:
  db_conn:
  - hostname: server.example.com
    port: 5432
    username: sa
    password: changeme
    metadata:
      run: ./build.sh
  top_level: key
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("array only", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
- ""
`
		dataValuesYAML := `#@data/values
---
- first
- second
`
		templateYAML := `#@ load("@ytt:data", "data")
---
rendered: #@ data.values
`
		expected := `rendered:
- first
- second
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
}

func TestNullableAnnotation(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("allows null on scalars", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  name: ""
  #@schema/nullable
  nullable_string: "empty"
  #@schema/nullable
  nullable_int: 10
  foo: ""
`
		dataValuesYAML := `#@data/values
---
vpc:
  name: vpc-203d912a
`
		templateYAML := `#@ load("@ytt:data", "data")
---
rendered: true
vpc: #@ data.values.vpc
`

		expected := `rendered: true
vpc:
  name: vpc-203d912a
  nullable_string: null
  nullable_int: null
  foo: ""
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("allows null on top level map item", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
#@schema/nullable
vpc:
  foo: "bar"
`
		dataValuesYAML := `---
#@data/values
---
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`

		expected := `vpc: null
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("allows null on map values", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  #@schema/nullable
  subnet_config:
  - id: 0
`
		dataValuesYAML := `#@data/values
---
vpc:
  subnet_config: ~
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`
		expected := `vpc:
  subnet_config: null
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("allows null on array values", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  #@schema/nullable
  subnet_config:
  - id: 0
    mask: "255.255.0.0"
    private: true
`
		dataValuesYAML := `#@data/values
---
vpc: {}
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`
		expected := `vpc:
  subnet_config: null
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("data values can override nullables", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  #@schema/nullable
  name: ""
`
		dataValuesYAML := `#@data/values
---
vpc:
  name: vpc-203d912a
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`
		expected := `vpc:
  name: vpc-203d912a
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
}

func TestDataValueNotConformingToSchemaFails(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("map value type mismatched", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
db_conn:
  port: 0
  username:
    main: "0"
`
		dataValuesYAML := `#@data/values
---
db_conn:
  port: localhost
  username:
    main: 123
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("data_values.yml", []byte(dataValuesYAML))),
		})
		expectedErr := `
data_values.yml:4 |   port: localhost
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: string
                  |   expected: integer (by schema.yml:4)


data_values.yml:6 |     main: 123
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: integer
                  |   expected: string (by schema.yml:6)
`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("array value type mismatched", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
clients:
- flags:
  - name: ""
`
		dataValuesYAML := `#@data/values
---
clients:
- flags: secure  #! expecting a array, got a string
- flags:
  - secure  #! expecting a map, got a string
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("data_values.yml", []byte(dataValuesYAML))),
		})

		expectedErr := `
data_values.yml:4 | - flags: secure  #! expecting a array, got a string
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: string
                  |   expected: array (by schema.yml:4)


data_values.yml:6 |   - secure  #! expecting a map, got a string
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: string
                  |   expected: map (by schema.yml:5)
`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("map key is not present in schema", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
db_conn:
  port: 0
`
		dataValuesYAML := `#@data/values
---
db_conn:
  port: not an int  #! wrong type, but check values only when all keys in the map are valid
  password: i should not be here
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("data_values.yml", []byte(dataValuesYAML))),
		})
		expectedErr := `data_values.yml:5 | password: i should not be here
                  |
                  | UNEXPECTED KEY - the key of this item was not found in the schema's corresponding map:
                  |      found: password
                  |   expected: (a key defined in map) (by schema.yml:3)
                  |   (hint: declare data values in schema and override them in a data values document)`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("null is given to a map item that is not nullable", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
app: 123
`
		dataValuesYAML := `#@data/values
---
app: null
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
		})
		expectedErr := `
dataValues.yml:3 | app: null
                 |
                 | TYPE MISMATCH - the value of this item is not what schema expected:
                 |      found: null
                 |   expected: integer (by schema.yml:3)`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("data values is given but schema is empty", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
`
		dataValuesYAML := `#@data/values
---
not_in_schema: "this should fail the type check!"
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", []byte(dataValuesYAML))),
		})
		expectedErr := "data values were found in data values file(s), but schema (schema.yml:2) has no values defined\n"
		expectedErr += "(hint: define matching keys from data values files(s) in the schema, or do not enable the schema feature)"

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("second data value conforms but the first data value does not conform", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
hostname: ""
`
		dataValuesYAML1 := `#@data/values
---
secret: super
`

		dataValuesYAML2 := `#@ load("@ytt:overlay", "overlay")
#@data/values
---
#@overlay/remove
secret:
`
		templateYAML := `---
rendered: true`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues1.yml", []byte(dataValuesYAML1))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues2.yml", []byte(dataValuesYAML2))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})
		expectedErr := `
dataValues1.yml:3 | secret: super
                  |
                  | UNEXPECTED KEY - the key of this item was not found in the schema's corresponding map:
                  |      found: secret
                  |   expected: (a key defined in map) (by schema.yml:2)
                  |   (hint: declare data values in schema and override them in a data values document)`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
}

func TestDefaultValuesAreFilledIn(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("values specified in the schema are the default data values", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
system_domain: "foo.domain"
`
		templateYAML := `#@ load("@ytt:data", "data")
---
system_domain: #@ data.values.system_domain
`
		expected := `system_domain: foo.domain
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})
		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("array defaults to an empty list", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  subnet_config:
  - id: 0
    mask: "255.255.0.0"
    private: true
`
		dataValuesYAML := `#@data/values
---
vpc: {}
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`
		expected := `vpc:
  subnet_config: []
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
	t.Run("when a key in the data value is omitted yet present in the schema, it is filled in", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  name: "name value"
  subnet_config:
  - id: 0
    mask: "255.255.0.0"
    private: true
`
		dataValuesYAML := `#@data/values
---
vpc:
  subnet_config:
  - id: 2
  - id: 3
    mask: 255.255.255.0
`
		templateYAML := `#@ load("@ytt:data", "data")
---
vpc: #@ data.values.vpc
`
		expected := `vpc:
  name: name value
  subnet_config:
  - id: 2
    mask: 255.255.0.0
    private: true
  - id: 3
    mask: 255.255.255.0
    private: true
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
}

func TestSchemaInLibraryModule(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true
	t.Run("eval respects schema as initial data value (Declarative Schema-Conforming Data Value)", func(t *testing.T) {
		configTplData := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
--- #@ template.replace(library.get("lib").eval())`)

		valuesData := []byte(`
#@library/ref "@lib"
#@data/values
---
foo: 7
`)

		libConfigTplData := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaData := []byte(`
#@schema/match data_values=True
---
foo: 42`)

		expectedYAMLTplData := `foo: 7
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", valuesData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config2.yml", libConfigTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaData)),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expectedYAMLTplData, opts)
	})
	t.Run("eval gives schema typecheck error (Declarative Schema-Non-Conforming Data Value)", func(t *testing.T) {
		configTplData := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
--- #@ template.replace(library.get("lib").eval())`)

		valuesData := []byte(`
#@library/ref "@lib"
#@data/values
---
foo: bar
`)

		libConfigTplData := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaData := []byte(`
#@schema/match data_values=True
---
foo: 42`)

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", valuesData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config2.yml", libConfigTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaData)),
		})

		expectedErr := `
- library.eval: Evaluating library 'lib': Overlaying data values (in following order: additional data values): 
    in <toplevel>
      config.yml:4 | --- #@ template.replace(library.get("lib").eval())

    reason:
     values.yml:5 | foo: bar
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: string
                  |   expected: integer (by _ytt_lib/lib/schema.yml:4)
     
     `
		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("with_data_values respects schema as initial data value (Programmatic Schema-Conforming Data Value)", func(t *testing.T) {
		configTplData := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
---
#@ def dvs_from_root():
#@overlay/match missing_ok=True
foo: from "root" library
#@ end
--- #@ template.replace(library.get("lib").with_data_values(dvs_from_root()).eval())`)

		libConfigTplData := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaData := []byte(`
#@schema/match data_values=True
---
foo: ""`)

		expectedYAMLTplData := `foo: from "root" library
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config2.yml", libConfigTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaData)),
		})

		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expectedYAMLTplData, opts)
	})
	t.Run("with_data_values gives schema typcheck error (Programmatic Schema-Non-Conforming Data Value)", func(t *testing.T) {
		configTplData := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
---
#@ def dvs_from_root():
#@overlay/match missing_ok=True
foo: 13
#@ end
--- #@ template.replace(library.get("lib").with_data_values(dvs_from_root()).eval())`)

		libConfigTplData := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaData := []byte(`
#@schema/match data_values=True
---
foo: ""`)

		expectedErr := `
- library.eval: Evaluating library 'lib': Overlaying data values (in following order: additional data values): 
    in <toplevel>
      config.yml:9 | --- #@ template.replace(library.get("lib").with_data_values(dvs_from_root()).eval())

    reason:
     config.yml:7 | foo: 13
                  |
                  | TYPE MISMATCH - the value of this item is not what schema expected:
                  |      found: integer
                  |   expected: string (by _ytt_lib/lib/schema.yml:4)
     
     `

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config2.yml", libConfigTplData)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaData)),
		})

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
}

func TestNoSchemaProvided(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("data value is given, provides an error and fails", func(t *testing.T) {
		dataValuesYAML := `#@data/values
---
db_conn:
`
		templateYAML := `---`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})
		expectedErr := "Schema feature is enabled but no schema document was provided"
		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("data value is not given, should succeed", func(t *testing.T) {
		templateYAML := `---
rendered: true`
		expected := `rendered: true
`
		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})
		assertYTTWorkflowSucceedsWithOutput(t, filesToProcess, expected, opts)
	})
}

func TestSchemaIsInvalidItFails(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("array value with fewer than one elements", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  subnet_ids: []
`
		dataValuesYAML := `#@data/values
---
vpc:
  subnet_ids:
  - 0
`
		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
		})
		expectedErr := `
schema.yml:4 |   subnet_ids: []
             |
             | INVALID ARRAY DEFINITION IN SCHEMA - unable to determine the desired type
             |      found: 0 array items
             |   expected: exactly 1 array item, of the desired type
             |   (hint: in a schema, the item of an array defines the type of its elements; its default value is an empty list)`
		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("array value with more than one elements", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  subnet_ids:
  - 0
  - 1
`
		dataValuesYAML := `#@data/values
---
vpc:
  subnet_ids: []
`
		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
		})
		expectedErr := `
schema.yml:4 |   subnet_ids:
             |
             | INVALID ARRAY DEFINITION IN SCHEMA - unable to determine the desired type
             |      found: 2 array items
             |   expected: exactly 1 array item, of the desired type
             |   (hint: to add elements to the default value of an array (i.e. an empty list), declare them in a @data/values document)`
		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("array value with a nullable annotation", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  subnet_ids:
  #@schema/nullable
  - 0
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
		})
		expectedErr := `
schema.yml:6 |   - 0
             |
             | INVALID SCHEMA - @schema/nullable is not supported on array items`

		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
	t.Run("null value", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
vpc:
  subnet_ids: null
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
		})
		expectedErr := `
schema.yml:4 |   subnet_ids: null
             |
             | INVALID SCHEMA - null value is not allowed in schema (no type can be inferred from it)
             |   (hint: to default to null, specify a value of the desired type and annotate with @schema/nullable)`
		assertYTTWorkflowFailsWithErrorMessage(t, filesToProcess, expectedErr, opts)
	})
}

func TestSchemaFeatureIsNotEnabledButSchemaIsPresentReportsAWarning(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = false
	stdout := bytes.NewBufferString("")
	stderr := bytes.NewBufferString("")
	ui := ui.NewCustomWriterTTY(false, stdout, stderr)

	schemaYAML := `#@schema/match data_values=True
---
`
	templateYAML := `---
rendered: true`

	filesToProcess := files.NewSortedFiles([]*files.File{
		files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
		files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
	})

	expectedStdErr := "Warning: schema document was detected, but schema experiment flag is not enabled. Did you mean to include --enable-experiment-schema?\n"
	expectedOut := "rendered: true\n"

	out := opts.RunWithFiles(cmdtpl.Input{Files: filesToProcess}, ui)
	if out.Err != nil {
		t.Fatalf("Expected RunWithFiles to succeed, but was error: %s", out.Err)
	}

	err := assertStdoutAndStderr(bytes.NewBuffer(out.Files[0].Bytes()), stderr, expectedOut, expectedStdErr)
	if err != nil {
		t.Fatalf("Assertion failed:\n %s", err)
	}
}

func assertYTTWorkflowSucceedsWithOutput(t *testing.T, filesToProcess []*files.File, expectedOut string, opts *cmdtpl.Options) {
	t.Helper()
	out := opts.RunWithFiles(cmdtpl.Input{Files: filesToProcess}, ui.NewTTY(false))
	if out.Err != nil {
		t.Fatalf("Expected RunWithFiles to succeed, but was error: %s", out.Err)
	}

	if len(out.Files) != 1 {
		t.Errorf("Expected number of output files to be 1, but was: %d", len(out.Files))
	}

	if string(out.Files[0].Bytes()) != expectedOut {
		diff := difflib.PPDiff(strings.Split(string(out.Files[0].Bytes()), "\n"), strings.Split(expectedOut, "\n"))
		t.Errorf("Expected output to only include template YAML, differences:\n%s", diff)
	}
}

func assertYTTWorkflowFailsWithErrorMessage(t *testing.T, filesToProcess []*files.File, expectedErr string, opts *cmdtpl.Options) {
	t.Helper()
	out := opts.RunWithFiles(cmdtpl.Input{Files: filesToProcess}, ui.NewTTY(false))
	if out.Err == nil {
		t.Fatalf("Expected an error, but succeeded.")
	}

	if !strings.Contains(out.Err.Error(), expectedErr) {
		diff := difflib.PPDiff(strings.Split(string(out.Err.Error()), "\n"), strings.Split(expectedErr, "\n"))
		t.Errorf("%s", diff)
	}
}
