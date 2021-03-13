// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"fmt"
	"strings"

	"github.com/k14s/ytt/pkg/template"
	"github.com/k14s/ytt/pkg/template/core"
	"github.com/k14s/ytt/pkg/yamlmeta"
)

const (
	AnnotationLibraryRef = "library/ref"

	dvsLibrarySep            = "@"
	dvsLibraryAliasIndicator = "~"
)

type LibRefPiece struct {
	Path  string
	Alias string
}

func (p LibRefPiece) Matches(lpp LibRefPiece) bool {
	pathMatch := p.Path == lpp.Path
	if p.Alias == "" {
		return pathMatch
	}

	aliasMatch := p.Alias == lpp.Alias
	if p.Path == "" {
		return aliasMatch
	}

	return aliasMatch && pathMatch
}

func (p LibRefPiece) AsString() string {
	if p.Alias == "" {
		return p.Path
	}
	return p.Path + dvsLibraryAliasIndicator + p.Alias
}

type DataValues struct {
	Doc         *yamlmeta.Document
	AfterLibMod bool
	used        bool

	originalLibRef []LibRefPiece
	libRef         []LibRefPiece
}

func NewDataValues(doc *yamlmeta.Document) (*DataValues, error) {
	_, libRef, afterLibMod, err := parseDVAnnotations(doc)
	if err != nil {
		return nil, err
	}

	return &DataValues{Doc: doc, AfterLibMod: afterLibMod, libRef: libRef, originalLibRef: libRef}, nil
}

func NewEmptyDataValues() *DataValues {
	return &DataValues{Doc: &yamlmeta.Document{}}
}

func NewDataValuesWithLib(doc *yamlmeta.Document, libRefStr string) (*DataValues, error) {
	libRef, err := parseLibRefStr(libRefStr)
	if err != nil {
		return nil, err
	}

	hasLibAnn, _, afterLibMod, err := parseDVAnnotations(doc)
	if err != nil {
		return nil, err
	} else if hasLibAnn {
		panic(fmt.Sprintf("Library was provided as arg as well as with %s annotation", AnnotationLibraryRef))
	}

	return &DataValues{Doc: doc, AfterLibMod: afterLibMod, libRef: libRef, originalLibRef: libRef}, nil
}

func NewDataValuesWithOptionalLib(doc *yamlmeta.Document, libRefStr string) (*DataValues, error) {
	if len(libRefStr) > 0 {
		return NewDataValuesWithLib(doc, libRefStr)
	}
	return NewDataValues(doc)
}

func (dvd *DataValues) IsUsed() bool { return dvd.used }
func (dvd *DataValues) markUsed()    { dvd.used = true }

func (dvd *DataValues) Desc() string {
	// TODO: Update to output file location of annotation. If no annotation use doc position although these will always be used
	var desc []string
	for _, refPiece := range dvd.originalLibRef {
		desc = append(desc, refPiece.AsString())
	}
	return fmt.Sprintf("library '%s%s' on %s", dvsLibrarySep,
		strings.Join(desc, dvsLibrarySep), dvd.Doc.Position.AsString())
}

func (dvd *DataValues) HasLibRef() bool { return len(dvd.libRef) > 0 }

func (dvd *DataValues) UsedInLibrary(expectedRefPiece LibRefPiece) *DataValues {
	if len(dvd.libRef) == 0 {
		dvd.markUsed()
		return dvd.deepCopy()
	}
	if !dvd.libRef[0].Matches(expectedRefPiece) {
		return nil
	}
	dvd.markUsed()
	childDV := dvd.deepCopy()
	childDV.libRef = childDV.libRef[1:]
	return childDV
}

func (dvd *DataValues) deepCopy() *DataValues {
	var copiedPieces []LibRefPiece
	copiedPieces = append(copiedPieces, dvd.libRef...)
	return &DataValues{Doc: dvd.Doc.DeepCopy(), AfterLibMod: dvd.AfterLibMod,
		libRef: copiedPieces, originalLibRef: dvd.originalLibRef}
}

func parseDVAnnotations(doc *yamlmeta.Document) (bool, []LibRefPiece, bool, error) {
	var libRef []LibRefPiece
	var hasLibAnn, afterLibMod bool

	anns := template.NewAnnotations(doc)

	if hasLibAnn = anns.Has(AnnotationLibraryRef); hasLibAnn {
		libArgs := anns.Args(AnnotationLibraryRef)
		if l := libArgs.Len(); l != 1 {
			return false, nil, false, fmt.Errorf("Expected %s annotation to have one arg, got %d", AnnotationDataValues, l)
		}

		argString, err := core.NewStarlarkValue(libArgs[0]).AsString()
		if err != nil {
			return false, nil, false, err
		}

		libRef, err = parseLibRefStr(argString)
		if err != nil {
			return false, nil, false, fmt.Errorf("Annotation %s: %s", AnnotationLibraryRef, err.Error())
		}
	}

	for _, kwarg := range anns.Kwargs(AnnotationDataValues) {
		kwargName, err := core.NewStarlarkValue(kwarg[0]).AsString()
		if err != nil {
			return false, nil, false, err
		}

		switch kwargName {
		case "after_library_module":
			afterLibMod, err = core.NewStarlarkValue(kwarg[1]).AsBool()
			if err != nil {
				return false, nil, false, err
			} else if len(libRef) == 0 {
				return false, nil, false, fmt.Errorf("Annotation %s: Expected kwarg 'after_library_module' to be used with %s annotation",
					AnnotationDataValues, AnnotationLibraryRef)
			}
		default:
			return false, nil, false, fmt.Errorf("Unknown kwarg %s for annotation %s", kwargName, AnnotationDataValues)
		}
	}
	return hasLibAnn, libRef, afterLibMod, nil
}

func parseLibRefStr(libRefStr string) ([]LibRefPiece, error) {
	if libRefStr == "" {
		return nil, fmt.Errorf("Expected library ref to not be empty")
	}

	if !strings.HasPrefix(libRefStr, dvsLibrarySep) {
		return nil, fmt.Errorf("Expected library ref to start with '%s'", dvsLibrarySep)
	}

	var result []LibRefPiece
	for _, refPiece := range strings.Split(libRefStr, dvsLibrarySep)[1:] {
		pathAndAlias := strings.Split(refPiece, dvsLibraryAliasIndicator)
		switch l := len(pathAndAlias); {
		case l == 1:
			result = append(result, LibRefPiece{Path: pathAndAlias[0]})

		case l == 2:
			if pathAndAlias[1] == "" {
				return nil, fmt.Errorf("Expected library alias to not be empty")
			}

			result = append(result, LibRefPiece{Path: pathAndAlias[0], Alias: pathAndAlias[1]})

		default:
			return nil, fmt.Errorf("Expected library ref to have form: '@path', '@~alias', or '@path~alias', got: '%s'", libRefStr)
		}
	}

	return result, nil
}
