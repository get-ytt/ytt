// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/k14s/starlark-go/starlark"
	"github.com/k14s/ytt/pkg/filepos"
	"github.com/k14s/ytt/pkg/template"
	"github.com/k14s/ytt/pkg/workspace"
	"github.com/k14s/ytt/pkg/yamlmeta"
	yttoverlay "github.com/k14s/ytt/pkg/yttlibrary/overlay"
	"github.com/spf13/cobra"
)

const (
	dvsKVSep     = "="
	dvsMapKeySep = "."
)

type DataValuesFlags struct {
	EnvFromStrings []string
	EnvFromYAML    []string

	KVsFromStrings []string
	KVsFromYAML    []string
	KVsFromFiles   []string

	Inspect bool

	EnvironFunc func() []string
}

func (s *DataValuesFlags) Set(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&s.EnvFromStrings, "data-values-env", nil, "Extract data values (as strings) from prefixed env vars (format: PREFIX for PREFIX_all__key1=str) (can be specified multiple times)")
	cmd.Flags().StringArrayVar(&s.EnvFromYAML, "data-values-env-yaml", nil, "Extract data values (parsed as YAML) from prefixed env vars (format: PREFIX for PREFIX_all__key1=true) (can be specified multiple times)")

	cmd.Flags().StringArrayVarP(&s.KVsFromStrings, "data-value", "v", nil, "Set specific data value to given value, as string (format: all.key1.subkey=123) (can be specified multiple times)")
	cmd.Flags().StringArrayVar(&s.KVsFromYAML, "data-value-yaml", nil, "Set specific data value to given value, parsed as YAML (format: all.key1.subkey=true) (can be specified multiple times)")
	cmd.Flags().StringArrayVar(&s.KVsFromFiles, "data-value-file", nil, "Set specific data value to given file contents, as string (format: all.key1.subkey=/file/path) (can be specified multiple times)")

	cmd.Flags().BoolVar(&s.Inspect, "data-values-inspect", false, "Inspect data values")
}

type dataValuesFlagsSource struct {
	Values        []string
	TransformFunc valueTransformFunc
}

type valueTransformFunc func(string) (interface{}, error)

func (s *DataValuesFlags) AsOverlays(strict bool) ([]*workspace.DataValues, []*workspace.DataValues, error) {
	plainValFunc := func(rawVal string) (interface{}, error) { return rawVal, nil }

	yamlValFunc := func(rawVal string) (interface{}, error) {
		val, err := s.parseYAML(rawVal, strict)
		if err != nil {
			return nil, fmt.Errorf("Deserializing YAML value: %s", err)
		}
		return val, nil
	}

	var result []*workspace.DataValues

	for _, src := range []dataValuesFlagsSource{{s.EnvFromStrings, plainValFunc}, {s.EnvFromYAML, yamlValFunc}} {
		for _, envPrefix := range src.Values {
			vals, err := s.env(envPrefix, src.TransformFunc)
			if err != nil {
				return nil, nil, fmt.Errorf("Extracting data values from env under prefix '%s': %s", envPrefix, err)
			}
			result = append(result, vals...)
		}
	}

	// KVs and files take precedence over environment variables
	for _, src := range []dataValuesFlagsSource{{s.KVsFromStrings, plainValFunc}, {s.KVsFromYAML, yamlValFunc}} {
		for _, kv := range src.Values {
			val, err := s.kv(kv, src.TransformFunc)
			if err != nil {
				return nil, nil, fmt.Errorf("Extracting data value from KV: %s", err)
			}
			result = append(result, val)
		}
	}

	for _, file := range s.KVsFromFiles {
		val, err := s.file(file)
		if err != nil {
			return nil, nil, fmt.Errorf("Extracting data value from file: %s", err)
		}
		result = append(result, val)
	}

	var overlayValues []*workspace.DataValues
	var libraryOverlays []*workspace.DataValues
	for _, doc := range result {
		if doc.HasLibRef() {
			libraryOverlays = append(libraryOverlays, doc)
		} else {
			overlayValues = append(overlayValues, doc)
		}
	}

	return overlayValues, libraryOverlays, nil
}

func (s *DataValuesFlags) env(prefix string, valueFunc valueTransformFunc) ([]*workspace.DataValues, error) {
	const (
		envKeyPrefix = "_"
		envMapKeySep = "__"
	)

	result := []*workspace.DataValues{}
	envVars := os.Environ()

	if s.EnvironFunc != nil {
		envVars = s.EnvironFunc()
	}

	libRef, keyPrefix, err := s.libraryRefAndKey(prefix)
	if err != nil {
		return nil, err
	}

	for _, envVar := range envVars {
		pieces := strings.SplitN(envVar, dvsKVSep, 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("Expected env variable to be key-value pair (format: key=value)")
		}

		if !strings.HasPrefix(pieces[0], keyPrefix+envKeyPrefix) {
			continue
		}

		val, err := valueFunc(pieces[1])
		if err != nil {
			return nil, fmt.Errorf("Extracting data value from env variable '%s': %s", pieces[0], err)
		}

		// '__' gets translated into a '.' since periods may not be liked by shells
		keyPieces := strings.Split(strings.TrimPrefix(pieces[0], keyPrefix+envKeyPrefix), envMapKeySep)
		overlay := s.buildOverlay(keyPieces, val, "env var")

		dvs, err := workspace.NewDataValuesWithOptionalLib(overlay, libRef)
		if err != nil {
			return nil, err
		}

		result = append(result, dvs)
	}

	return result, nil
}

func (s *DataValuesFlags) kv(kv string, valueFunc valueTransformFunc) (*workspace.DataValues, error) {
	pieces := strings.SplitN(kv, dvsKVSep, 2)
	if len(pieces) != 2 {
		return nil, fmt.Errorf("Expected format key=value")
	}

	val, err := valueFunc(pieces[1])
	if err != nil {
		return nil, fmt.Errorf("Deserializing value for key '%s': %s", pieces[0], err)
	}

	libRef, key, err := s.libraryRefAndKey(pieces[0])
	if err != nil {
		return nil, err
	}

	overlay := s.buildOverlay(strings.Split(key, dvsMapKeySep), val, "kv arg")

	return workspace.NewDataValuesWithOptionalLib(overlay, libRef)
}

func (s *DataValuesFlags) parseYAML(data string, strict bool) (interface{}, error) {
	docSet, err := yamlmeta.NewParser(yamlmeta.ParserOpts{Strict: strict}).ParseBytes([]byte(data), "")
	if err != nil {
		return nil, err
	}
	return docSet.Items[0].Value, nil
}

func (s *DataValuesFlags) file(kv string) (*workspace.DataValues, error) {
	pieces := strings.SplitN(kv, dvsKVSep, 2)
	if len(pieces) != 2 {
		return nil, fmt.Errorf("Expected format key=/file/path")
	}

	contents, err := ioutil.ReadFile(pieces[1])
	if err != nil {
		return nil, fmt.Errorf("Reading file '%s'", pieces[1])
	}

	libRef, key, err := s.libraryRefAndKey(pieces[0])
	if err != nil {
		return nil, err
	}

	overlay := s.buildOverlay(strings.Split(key, dvsMapKeySep), string(contents), "key=file arg")

	return workspace.NewDataValuesWithOptionalLib(overlay, libRef)
}

func (DataValuesFlags) libraryRefAndKey(key string) (string, string, error) {
	const (
		libraryKeySep = ":"
	)

	keyPieces := strings.Split(key, libraryKeySep)

	switch len(keyPieces) {
	case 1:
		return "", key, nil

	case 2:
		if len(keyPieces[0]) == 0 {
			return "", "", fmt.Errorf("Expected library ref to not be empty")
		}
		return keyPieces[0], keyPieces[1], nil

	default:
		return "", "", fmt.Errorf("Expected at most one library-key separator '%s' in '%s'", libraryKeySep, key)
	}
}

func (s *DataValuesFlags) buildOverlay(keyPieces []string, value interface{}, desc string) *yamlmeta.Document {
	const (
		missingOkSuffix = "+"
	)

	resultMap := &yamlmeta.Map{}
	currMap := resultMap
	var lastMapItem *yamlmeta.MapItem

	pos := filepos.NewPosition(1)
	pos.SetFile(fmt.Sprintf("key '%s' (%s)", strings.Join(keyPieces, dvsMapKeySep), desc))

	for _, piece := range keyPieces {
		newMap := &yamlmeta.Map{}
		nodeAnns := template.NodeAnnotations{}

		if strings.HasSuffix(piece, missingOkSuffix) {
			piece = piece[:len(piece)-1]
			nodeAnns = template.NodeAnnotations{
				yttoverlay.AnnotationMatch: template.NodeAnnotation{
					Kwargs: []starlark.Tuple{{
						starlark.String(yttoverlay.MatchAnnotationKwargMissingOK),
						starlark.Bool(true),
					}},
				},
			}
		}

		lastMapItem = &yamlmeta.MapItem{Key: piece, Value: newMap, Position: pos}
		lastMapItem.SetAnnotations(nodeAnns)

		currMap.Items = append(currMap.Items, lastMapItem)
		currMap = newMap
	}

	lastMapItem.Value = yamlmeta.NewASTFromInterface(value)

	// Explicitly replace entire value at given key
	// (this allows to specify non-scalar data values)
	existingAnns := template.NewAnnotations(lastMapItem)
	existingAnns[yttoverlay.AnnotationReplace] = template.NodeAnnotation{}
	lastMapItem.SetAnnotations(existingAnns)

	return &yamlmeta.Document{Value: resultMap, Position: pos}
}
