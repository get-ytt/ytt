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

func TestSchema_Passes_when_DataValues_conform(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("when document's value is a map", func(t *testing.T) {
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

		assertSucceeds(t, filesToProcess, expected, opts)
	})
	t.Run("when document's value is an array", func(t *testing.T) {
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

		assertSucceeds(t, filesToProcess, expected, opts)
	})
	t.Run("when document's value is a scalar", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
42
`
		dataValuesYAML := `#@data/values
---
13
`
		templateYAML := `#@ load("@ytt:data", "data")
---
data_value: #@ data.values
`
		expected := "data_value: 13\n"

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertSucceeds(t, filesToProcess, expected, opts)
	})

	t.Run("when neither schema nor data values are given", func(t *testing.T) {
		assertSucceeds(t,
			files.NewSortedFiles([]*files.File{
				files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte("true"))),
			}),
			"true\n", opts)
	})
}

func TestSchema_Reports_violations_when_DataValues_do_NOT_conform(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("when map item's key is not among those declared in schema", func(t *testing.T) {
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

		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("when map item's value is the wrong type", func(t *testing.T) {
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

		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("when map item's value is null but is not nullable", func(t *testing.T) {
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

		assertFails(t, filesToProcess, expectedErr, opts)
	})

	t.Run("when array item's value is the wrong type", func(t *testing.T) {
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

		assertFails(t, filesToProcess, expectedErr, opts)
	})

	t.Run("when schema is null and non-empty data values is given", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
`
		dataValuesYAML := `#@data/values
---
foo: non-empty data value
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", []byte(dataValuesYAML))),
		})
		expectedErr := "data values were found in data values file(s), but schema (schema.yml:2) has no values defined\n"
		expectedErr += "(hint: define matching keys from data values files(s) in the schema, or do not enable the schema feature)"

		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("checks after every data values document is processed (and stops if there was a violation)", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
hostname: ""
`
		nonConformingDataValueYAML := `#@data/values
---
not_in_schema: this should be the only violation reported
`

		wouldFixNonConformingDataValueYAML := `#@ load("@ytt:overlay", "overlay")
#@data/values
---
#@overlay/remove
not_in_schema:
`
		templateYAML := `---
rendered: true`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("nonConforming.yml", []byte(nonConformingDataValueYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("fixNonConforming.yml", []byte(wouldFixNonConformingDataValueYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})
		expectedErr := `
nonConforming.yml:3 | not_in_schema: this should be the only violation reported
                    |
                    | UNEXPECTED KEY - the key of this item was not found in the schema's corresponding map:
                    |      found: not_in_schema
                    |   expected: (a key defined in map) (by schema.yml:2)
                    |   (hint: declare data values in schema and override them in a data values document)`

		assertFails(t, filesToProcess, expectedErr, opts)
	})
}

func TestSchema_Provides_default_values(t *testing.T) {
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
		assertSucceeds(t, filesToProcess, expected, opts)
	})
	t.Run("array default to an empty list", func(t *testing.T) {
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

		assertSucceeds(t, filesToProcess, expected, opts)
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

		assertSucceeds(t, filesToProcess, expected, opts)
	})
}

func TestSchema_Allows_null_values_via_nullable_annotation(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("when the value is a map", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
defaults:
  #@schema/nullable
  contains_map:
    a: 1
    b: 2
overriden:
  #@schema/nullable
  contains_map:
    a: 1
    b: 1

`
		dataValuesYAML := `#@data/values
---
overriden:
  contains_map:
    b: 2
`
		templateYAML := `#@ load("@ytt:data", "data")
---
defaults: #@ data.values.defaults
overriden: #@ data.values.overriden
`
		expected := `defaults:
  contains_map: null
overriden:
  contains_map:
    b: 2
    a: 1
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertSucceeds(t, filesToProcess, expected, opts)
	})
	t.Run("when the value is a array", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
defaults:
  #@schema/nullable
  contains_array:
  - ""
overriden:
  #@schema/nullable
  contains_array:
  - a: 1
    b: 0
`
		dataValuesYAML := `#@data/values
---
overriden:
  contains_array:
  - a: 20
`
		templateYAML := `#@ load("@ytt:data", "data")
---
defaults: #@ data.values.defaults
overriden: #@ data.values.overriden
`
		expected := `defaults:
  contains_array: null
overriden:
  contains_array:
  - a: 20
    b: 0
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertSucceeds(t, filesToProcess, expected, opts)
	})
	t.Run("when the value is a scalar", func(t *testing.T) {
		schemaYAML := `#@schema/match data_values=True
---
defaults:
  #@schema/nullable
  nullable_string: "empty"
  #@schema/nullable
  nullable_int: 10
  #@schema/nullable
  nullable_bool: true
overriden:
  #@schema/nullable
  nullable_string: "empty"
  #@schema/nullable
  nullable_int: 10
  #@schema/nullable
  nullable_bool: false
`
		dataValuesYAML := `#@data/values
---
overriden:
  nullable_string: set from data value
  nullable_int: 42
  nullable_bool: true
`
		templateYAML := `#@ load("@ytt:data", "data")
---
defaults: #@ data.values.defaults
overriden: #@ data.values.overriden
`

		expected := `defaults:
  nullable_string: null
  nullable_int: null
  nullable_bool: null
overriden:
  nullable_string: set from data value
  nullable_int: 42
  nullable_bool: true
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", []byte(schemaYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("dataValues.yml", []byte(dataValuesYAML))),
			files.MustNewFileFromSource(files.NewBytesSource("template.yml", []byte(templateYAML))),
		})

		assertSucceeds(t, filesToProcess, expected, opts)
	})
}

func TestSchema_Is_scoped_to_a_library(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("when data values are ref'ed to a library, they are only checked by that library's schema", func(t *testing.T) {
		configYAML := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
--- #@ template.replace(library.get("lib").eval())`)

		valuesYAML := []byte(`
#@library/ref "@lib"
#@data/values
---
in_library: 7
`)

		schemaYAML := []byte(`
#@schema/match data_values=True
---
in_root_library: ""
`)

		libConfigYAML := []byte(`
#@ load("@ytt:data", "data")
---
lib_data_values: #@ data.values.in_library`)

		libSchemaData := []byte(`
#@schema/match data_values=True
---
in_library: 0`)

		expectedYAMLTplData := `lib_data_values: 7
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", valuesYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", schemaYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config.yml", libConfigYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaData)),
		})

		assertSucceeds(t, filesToProcess, expectedYAMLTplData, opts)
	})
	t.Run("when data values are programmatically set on a library, they are only checked by that library's schema", func(t *testing.T) {
		configYAML := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
---
#@ def dvs_from_root():
foo: from "root" library
#@ end
--- #@ template.replace(library.get("lib").with_data_values(dvs_from_root()).eval())`)

		schemaYAML := []byte(`
#@schema/match data_values=True
---
foo: 0`)

		libConfigYAML := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaYAML := []byte(`
#@schema/match data_values=True
---
foo: ""`)

		expectedYAMLTplData := `foo: from "root" library
`

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("schema.yml", schemaYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config.yml", libConfigYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaYAML)),
		})

		assertSucceeds(t, filesToProcess, expectedYAMLTplData, opts)
	})
	t.Run("when a library is evaluated, schema violations are reported", func(t *testing.T) {
		configYAML := []byte(`
#@ load("@ytt:template", "template")
#@ load("@ytt:library", "library")
--- #@ template.replace(library.get("lib").eval())`)

		valuesYAML := []byte(`
#@library/ref "@lib"
#@data/values
---
foo: bar
`)

		libConfigYAML := []byte(`
#@ load("@ytt:data", "data")
---
foo: #@ data.values.foo`)

		libSchemaYAML := []byte(`
#@schema/match data_values=True
---
foo: 42`)

		filesToProcess := files.NewSortedFiles([]*files.File{
			files.MustNewFileFromSource(files.NewBytesSource("config.yml", configYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("values.yml", valuesYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/config.yml", libConfigYAML)),
			files.MustNewFileFromSource(files.NewBytesSource("_ytt_lib/lib/schema.yml", libSchemaYAML)),
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
		assertFails(t, filesToProcess, expectedErr, opts)
	})
}

func TestSchema_When_invalid_reports_error(t *testing.T) {
	opts := cmdtpl.NewOptions()
	opts.SchemaEnabled = true

	t.Run("array with fewer than one element", func(t *testing.T) {
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
		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("array with more than one element", func(t *testing.T) {
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
		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("array with a nullable annotation", func(t *testing.T) {
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

		assertFails(t, filesToProcess, expectedErr, opts)
	})
	t.Run("item with null value", func(t *testing.T) {
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
		assertFails(t, filesToProcess, expectedErr, opts)
	})
}

func TestSchema_Warns_when_feature_disabled_and_schema_provided(t *testing.T) {
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

func assertSucceeds(t *testing.T, filesToProcess []*files.File, expectedOut string, opts *cmdtpl.Options) {
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

func assertFails(t *testing.T, filesToProcess []*files.File, expectedErr string, opts *cmdtpl.Options) {
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
