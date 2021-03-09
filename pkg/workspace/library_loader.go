// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"fmt"
	"strings"

	"github.com/k14s/ytt/pkg/cmd/ui"

	"github.com/k14s/starlark-go/starlark"
	"github.com/k14s/ytt/pkg/files"
	"github.com/k14s/ytt/pkg/schema"
	"github.com/k14s/ytt/pkg/structmeta"
	"github.com/k14s/ytt/pkg/yamlmeta"
	"github.com/k14s/ytt/pkg/yamltemplate"
)

type LibraryLoader struct {
	libraryCtx         LibraryExecutionContext
	ui                 ui.UI
	templateLoaderOpts TemplateLoaderOpts
	libraryExecFactory *LibraryExecutionFactory
}

type EvalResult struct {
	Files   []files.OutputFile
	DocSet  *yamlmeta.DocumentSet
	Exports []EvalExport
}

type EvalExport struct {
	Path    string
	Symbols starlark.StringDict
}

func NewLibraryLoader(libraryCtx LibraryExecutionContext,
	ui ui.UI, templateLoaderOpts TemplateLoaderOpts,
	libraryExecFactory *LibraryExecutionFactory) *LibraryLoader {

	return &LibraryLoader{
		libraryCtx:         libraryCtx,
		ui:                 ui,
		templateLoaderOpts: templateLoaderOpts,
		libraryExecFactory: libraryExecFactory,
	}
}

func (ll *LibraryLoader) Schemas() ([]*yamlmeta.Document, error) {
	loader := NewTemplateLoader(NewEmptyDataValues(), nil, ll.ui, ll.templateLoaderOpts, ll.libraryExecFactory, &schema.AnySchema{})

	schemaFiles, err := ll.schemaFiles(loader)
	if err != nil {
		return nil, err
	}

	if len(schemaFiles) > 0 {
		libraryCtx := LibraryExecutionContext{Current: schemaFiles[0].Library, Root: NewRootLibrary(nil)}

		_, resultDocSet, err := loader.EvalYAML(libraryCtx, schemaFiles[0].File)
		if err != nil {
			return nil, err
		}

		tplOpts := yamltemplate.MetasOpts{IgnoreUnknown: ll.templateLoaderOpts.IgnoreUnknownComments}

		docs, _, err := DocExtractor{resultDocSet, tplOpts}.Extract(AnnotationSchemaMatch)
		if err != nil {
			return nil, err
		}

		return docs, nil
	}
	return nil, nil
}

func (ll *LibraryLoader) Values(valuesOverlays []*DataValues, schema Schema) (*DataValues, []*DataValues, error) {
	loader := NewTemplateLoader(NewEmptyDataValues(), nil, ll.ui, ll.templateLoaderOpts, ll.libraryExecFactory, schema)

	valuesFiles, err := ll.valuesFiles(loader)
	if err != nil {
		return nil, nil, err
	}

	dvpp := DataValuesPreProcessing{
		valuesFiles:           valuesFiles,
		valuesOverlays:        valuesOverlays,
		loader:                loader,
		IgnoreUnknownComments: ll.templateLoaderOpts.IgnoreUnknownComments,
	}

	return dvpp.Apply()
}

func (ll *LibraryLoader) schemaFiles(loader *TemplateLoader) ([]*FileInLibrary, error) {
	return ll.filesByAnnotation(AnnotationSchemaMatch, loader)
}

func (ll *LibraryLoader) valuesFiles(loader *TemplateLoader) ([]*FileInLibrary, error) {
	return ll.filesByAnnotation(AnnotationDataValues, loader)

}

func (ll *LibraryLoader) filesByAnnotation(annName structmeta.AnnotationName, loader *TemplateLoader) ([]*FileInLibrary, error) {
	var valuesFiles []*FileInLibrary

	for _, fileInLib := range ll.libraryCtx.Current.ListAccessibleFiles() {
		if fileInLib.File.Type() == files.TypeYAML && fileInLib.File.IsTemplate() {
			docSet, err := loader.ParseYAML(fileInLib.File)
			if err != nil {
				return nil, err
			}

			tplOpts := yamltemplate.MetasOpts{IgnoreUnknown: ll.templateLoaderOpts.IgnoreUnknownComments}

			values, _, err := DocExtractor{docSet, tplOpts}.Extract(annName)
			if err != nil {
				return nil, err
			}

			if len(values) > 0 {
				valuesFiles = append(valuesFiles, fileInLib)
				fileInLib.File.MarkForOutput(false)
			}
		}
	}

	return valuesFiles, nil
}

func (ll *LibraryLoader) Eval(values *DataValues, libraryValues []*DataValues) (*EvalResult, error) {
	exports, docSets, outputFiles, err := ll.eval(values, libraryValues)
	if err != nil {
		return nil, err
	}

	docSets, err = (&OverlayPostProcessing{docSets: docSets}).Apply()
	if err != nil {
		return nil, err
	}

	result := &EvalResult{
		Files:   outputFiles,
		DocSet:  &yamlmeta.DocumentSet{},
		Exports: exports,
	}

	for _, fileInLib := range ll.sortedOutputDocSets(docSets) {
		docSet := docSets[fileInLib]
		result.DocSet.Items = append(result.DocSet.Items, docSet.Items...)

		resultDocBytes, err := docSet.AsBytes()
		if err != nil {
			return nil, fmt.Errorf("Marshaling template result: %s", err)
		}

		ll.ui.Debugf("### %s result\n%s", fileInLib.RelativePath(), resultDocBytes)
		result.Files = append(result.Files, files.NewOutputFile(fileInLib.RelativePath(), resultDocBytes, fileInLib.File.Type()))
	}

	return result, nil
}

func (ll *LibraryLoader) eval(values *DataValues, libraryValues []*DataValues) ([]EvalExport,
	map[*FileInLibrary]*yamlmeta.DocumentSet, []files.OutputFile, error) {

	loader := NewTemplateLoader(values, libraryValues, ll.ui, ll.templateLoaderOpts, ll.libraryExecFactory, &schema.AnySchema{})

	exports := []EvalExport{}
	docSets := map[*FileInLibrary]*yamlmeta.DocumentSet{}
	outputFiles := []files.OutputFile{}

	for _, fileInLib := range ll.libraryCtx.Current.ListAccessibleFiles() {
		libraryCtx := LibraryExecutionContext{Current: fileInLib.Library, Root: ll.libraryCtx.Root}

		switch {
		case fileInLib.File.IsForOutput():
			// Do not collect globals produced by templates
			switch fileInLib.File.Type() {
			case files.TypeYAML:
				_, resultDocSet, err := loader.EvalYAML(libraryCtx, fileInLib.File)
				if err != nil {
					return nil, nil, nil, err
				}

				docSets[fileInLib] = resultDocSet

			case files.TypeText:
				_, resultVal, err := loader.EvalText(libraryCtx, fileInLib.File)
				if err != nil {
					return nil, nil, nil, err
				}

				resultStr := resultVal.AsString()

				ll.ui.Debugf("### %s result\n%s", fileInLib.RelativePath(), resultStr)
				outputFiles = append(outputFiles, files.NewOutputFile(fileInLib.RelativePath(), []byte(resultStr), fileInLib.File.Type()))

			default:
				return nil, nil, nil, fmt.Errorf("Unknown file type")
			}

		case fileInLib.File.IsLibrary():
			// Collect globals produced by library files
			var evalFunc func(LibraryExecutionContext, *files.File) (starlark.StringDict, error)

			switch fileInLib.File.Type() {
			case files.TypeYAML:
				evalFunc = func(libraryCtx LibraryExecutionContext, file *files.File) (starlark.StringDict, error) {
					globals, _, err := loader.EvalYAML(libraryCtx, fileInLib.File)
					return globals, err
				}

			case files.TypeText:
				evalFunc = func(libraryCtx LibraryExecutionContext, file *files.File) (starlark.StringDict, error) {
					globals, _, err := loader.EvalText(libraryCtx, fileInLib.File)
					return globals, err
				}

			case files.TypeStarlark:
				evalFunc = loader.EvalStarlark

			default:
				// TODO should we allow skipping over unknown library files?
				// do nothing
			}

			if evalFunc != nil {
				globals, err := evalFunc(libraryCtx, fileInLib.File)
				if err != nil {
					return nil, nil, nil, err
				}

				exports = append(exports, EvalExport{Path: fileInLib.RelativePath(), Symbols: globals})
			}

		default:
			// do nothing
		}
	}

	return exports, docSets, outputFiles, ll.checkUnusedDVs(libraryValues)
}

func (*LibraryLoader) sortedOutputDocSets(outputDocSets map[*FileInLibrary]*yamlmeta.DocumentSet) []*FileInLibrary {
	var files []*FileInLibrary
	for file := range outputDocSets {
		files = append(files, file)
	}
	SortFilesInLibrary(files)
	return files
}

func (LibraryLoader) checkUnusedDVs(libraryValues []*DataValues) error {
	var unusedValuesDescs []string
	for _, dv := range libraryValues {
		if !dv.IsUsed() {
			unusedValuesDescs = append(unusedValuesDescs, dv.Desc())
		}
	}

	if len(unusedValuesDescs) == 0 {
		return nil
	}

	return fmt.Errorf("Expected all provided library data values documents "+
		"to be used but found unused: %s", strings.Join(unusedValuesDescs, ", "))
}
