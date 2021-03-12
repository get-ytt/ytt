// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"fmt"
	"strings"

	"github.com/k14s/starlark-go/starlark"
	"github.com/k14s/ytt/pkg/filepos"
	"github.com/k14s/ytt/pkg/schema"
	"github.com/k14s/ytt/pkg/template"
	"github.com/k14s/ytt/pkg/yamlmeta"
	"github.com/k14s/ytt/pkg/yamltemplate"
	yttoverlay "github.com/k14s/ytt/pkg/yttlibrary/overlay"
)

type DataValuesPreProcessing struct {
	valuesFiles           []*FileInLibrary
	valuesOverlays        []*DataValues
	loader                *TemplateLoader
	IgnoreUnknownComments bool // TODO remove?
}

func (o DataValuesPreProcessing) Apply() (*DataValues, []*DataValues, error) {
	files := append([]*FileInLibrary{}, o.valuesFiles...)

	// Respect assigned file order for data values overlaying to succeed
	SortFilesInLibrary(files)

	dataValues, libraryDataValues, err := o.apply(files)
	if err != nil {
		errMsg := "Overlaying data values (in following order: %s): %s"
		return nil, nil, fmt.Errorf(errMsg, o.allFileDescs(files), err)
	}

	return dataValues, libraryDataValues, nil
}

func (o DataValuesPreProcessing) apply(files []*FileInLibrary) (*DataValues, []*DataValues, error) {
	values := o.loader.schema.AsDataValue()
	var libraryValues []*DataValues
	for _, fileInLib := range files {
		valuesDocs, err := o.templateFile(fileInLib)
		if err != nil {
			return nil, nil, fmt.Errorf("Templating file '%s': %s", fileInLib.File.RelativePath(), err)
		}

		for _, valuesDoc := range valuesDocs {
			dv, err := NewDataValues(valuesDoc)
			if err != nil {
				return nil, nil, err
			}

			switch {
			case dv.HasLib():
				libraryValues = append(libraryValues, dv)
			case values == nil:
				// Confirmed presence of non private lib data value
				// if schema is a NullSchema, error in due to root lvl data value with no root lvl schema
				err := o.loader.schema.ValidateWithValues(1)
				if err != nil {
					return nil, nil, err
				}

				values = valuesDoc
			default:
				var err error

				if _, ok := o.loader.schema.(*schema.DocumentSchema); ok {
					anns := template.NewAnnotations(dv.Doc)
					_, defaultsSetAlready := anns[yttoverlay.AnnotationMatchChildDefaults]
					if !defaultsSetAlready {
						anns[yttoverlay.AnnotationMatchChildDefaults] = template.NodeAnnotation{
							Kwargs: []starlark.Tuple{{
								starlark.String(yttoverlay.MatchAnnotationKwargMissingOK),
								starlark.Bool(true),
							}},
						}
					}
					dv.Doc.SetAnnotations(anns)
				}

				values, err = o.overlay(values, dv.Doc)
				if err != nil {
					return nil, nil, err
				}
			}
		}

		if _, ok := o.loader.schema.(*schema.DocumentSchema); ok {
			outerTypeCheck := o.loader.schema.AssignType(values)
			if len(outerTypeCheck.Violations) > 0 {
				return nil, nil, outerTypeCheck
			}

			typeCheck := values.Check()
			outerTypeCheck.Violations = append(outerTypeCheck.Violations, typeCheck.Violations...)

			if len(outerTypeCheck.Violations) > 0 {
				return nil, nil, outerTypeCheck
			}
		}
	}

	values, err := o.overlayValuesOverlays(values)
	if err != nil {
		return nil, nil, err
	}

	dv, err := NewDataValues(values)
	if err != nil {
		return nil, nil, err
	}

	return dv, libraryValues, nil
}

func (o DataValuesPreProcessing) allFileDescs(files []*FileInLibrary) string {
	var result []string
	for _, fileInLib := range files {
		result = append(result, fileInLib.File.RelativePath())
	}
	if len(o.valuesOverlays) > 0 {
		result = append(result, "additional data values")
	}
	return strings.Join(result, ", ")
}

func (o DataValuesPreProcessing) templateFile(fileInLib *FileInLibrary) ([]*yamlmeta.Document, error) {
	libraryCtx := LibraryExecutionContext{Current: fileInLib.Library, Root: NewRootLibrary(nil)}

	_, resultDocSet, err := o.loader.EvalYAML(libraryCtx, fileInLib.File)
	if err != nil {
		return nil, err
	}

	tplOpts := yamltemplate.MetasOpts{IgnoreUnknown: o.IgnoreUnknownComments}

	// Extract _all_ data values docs from the templated result
	valuesDocs, nonValuesDocs, err := DocExtractor{resultDocSet, tplOpts}.Extract(AnnotationDataValues)
	if err != nil {
		return nil, err
	}

	// Fail if there any non-empty docs that are not data values
	if len(nonValuesDocs) > 0 {
		for _, doc := range nonValuesDocs {
			if !doc.IsEmpty() {
				errStr := "Expected data values file '%s' to only have data values documents"
				return nil, fmt.Errorf(errStr, fileInLib.File.RelativePath())
			}
		}
	}

	return valuesDocs, nil
}

func (o DataValuesPreProcessing) overlay(valuesDoc, newValuesDoc *yamlmeta.Document) (*yamlmeta.Document, error) {
	op := yttoverlay.Op{
		Left:   &yamlmeta.DocumentSet{Items: []*yamlmeta.Document{valuesDoc}},
		Right:  &yamlmeta.DocumentSet{Items: []*yamlmeta.Document{newValuesDoc}},
		Thread: &starlark.Thread{Name: "data-values-pre-processing"},

		ExactMatch: true,
	}

	newLeft, err := op.Apply()
	if err != nil {
		return nil, err
	}

	return newLeft.(*yamlmeta.DocumentSet).Items[0], nil
}

func (o DataValuesPreProcessing) overlayValuesOverlays(valuesDoc *yamlmeta.Document) (*yamlmeta.Document, error) {
	if valuesDoc == nil {
		// TODO get rid of assumption that data values is a map?
		valuesDoc = &yamlmeta.Document{
			Value:    &yamlmeta.Map{},
			Position: filepos.NewUnknownPosition(),
		}
	}

	if _, ok := o.loader.schema.(*schema.DocumentSchema); ok {
		// loop through values overlays to ensure they conform to schema
		for _, dv := range o.valuesOverlays {
			var typeCheck yamlmeta.TypeCheck

			typeCheck = o.loader.schema.AssignType(dv.Doc)
			if len(typeCheck.Violations) > 0 {
				return nil, typeCheck
			}

			typeCheck = dv.Doc.Check()
			if len(typeCheck.Violations) > 0 {
				return nil, typeCheck
			}
		}
	}
	var result *yamlmeta.Document

	// by default return itself
	result = valuesDoc

	for _, valuesOverlay := range o.valuesOverlays {
		var err error

		result, err = o.overlay(result, valuesOverlay.Doc)
		if err != nil {
			// TODO improve error message?
			return nil, fmt.Errorf("Overlaying additional data values on top of "+
				"data values from files (marked as @data/values): %s", err)
		}
	}

	return result, nil
}
