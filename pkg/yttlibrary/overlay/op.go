// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package overlay

import (
	"fmt"
	// "os" // yamlmeta.NewPrinter(os.Stdout).Print(typedLeft)

	"github.com/k14s/starlark-go/starlark"
	"github.com/k14s/ytt/pkg/template"
	"github.com/k14s/ytt/pkg/yamlmeta"
)

type Op struct {
	Left  interface{}
	Right interface{}

	Thread *starlark.Thread

	ExactMatch bool
}

func (o Op) Apply() (interface{}, error) {
	leftObj := yamlmeta.NewASTFromInterface(o.Left)
	rightObj := yamlmeta.NewASTFromInterface(o.Right)

	_, err := o.apply(leftObj, rightObj, NewEmptyMatchChildDefaultsAnnotation())
	if err != nil {
		return nil, err
	}

	o.removeOverlayAnns(leftObj)
	return leftObj, nil
}

func (o Op) apply(left, right interface{}, parentMatchChildDefaults MatchChildDefaultsAnnotation) (bool, error) {
	switch typedRight := right.(type) {
	case *yamlmeta.DocumentSet:
		var docSetArray []*yamlmeta.DocumentSet

		typedLeft, isDocSet := left.(*yamlmeta.DocumentSet)
		if !isDocSet {
			// support array of docsets to allow consumers to
			// keep proper association of document to docsets
			// (see matching for overlay post processing)
			typedLeft, isDocSetArray := left.([]*yamlmeta.DocumentSet)
			if !isDocSetArray {
				return false, fmt.Errorf("Expected docset, but was %T", left)
			}
			docSetArray = typedLeft
		} else {
			docSetArray = []*yamlmeta.DocumentSet{typedLeft}
		}

		return o.applyDocSet(docSetArray, typedRight, parentMatchChildDefaults)

	case *yamlmeta.Document:
		panic("Unexpected doc")

	case *yamlmeta.Map:
		typedLeft, isMap := left.(*yamlmeta.Map)
		if !isMap {
			return false, fmt.Errorf("Expected map, but was %T", left)
		}

		for _, item := range typedRight.Items {
			item := item.DeepCopy()

			op, err := whichOp(item)
			if err == nil {
				switch op {
				case AnnotationMerge:
					err = o.mergeMapItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationRemove:
					err = o.removeMapItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationReplace:
					err = o.replaceMapItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationAssert:
					err = o.assertMapItem(typedLeft, item, parentMatchChildDefaults)
				default:
					err = fmt.Errorf("Overlay op %s is not supported on map item", op)
				}
			}
			if err != nil {
				return false, fmt.Errorf("Map item (key '%s') on %s: %s",
					item.Key, item.Position.AsString(), err)
			}
		}

	case *yamlmeta.MapItem:
		panic("Unexpected mapitem")

	case *yamlmeta.Array:
		typedLeft, isArray := left.(*yamlmeta.Array)
		if !isArray {
			return false, fmt.Errorf("Expected array, but was %T", left)
		}

		for _, item := range typedRight.Items {
			item := item.DeepCopy()

			op, err := whichOp(item)
			if err == nil {
				switch op {
				case AnnotationMerge:
					err = o.mergeArrayItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationRemove:
					err = o.removeArrayItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationReplace:
					err = o.replaceArrayItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationInsert:
					err = o.insertArrayItem(typedLeft, item, parentMatchChildDefaults)
				case AnnotationAppend:
					err = o.appendArrayItem(typedLeft, item)
				case AnnotationAssert:
					err = o.assertArrayItem(typedLeft, item, parentMatchChildDefaults)
				default:
					err = fmt.Errorf("Overlay op %s is not supported on array item", op)
				}
			}
			if err != nil {
				return false, fmt.Errorf("Array item on %s: %s", item.Position.AsString(), err)
			}
		}

	case *yamlmeta.ArrayItem:
		panic("Unexpected arrayitem")

	default:
		return true, nil
	}

	return false, nil
}

func (o Op) applyDocSet(
	typedLeft []*yamlmeta.DocumentSet, typedRight *yamlmeta.DocumentSet,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) (bool, error) {

	for _, doc := range typedRight.Items {
		doc := doc.DeepCopy()

		op, err := whichOp(doc)
		if err == nil {
			switch op {
			case AnnotationMerge:
				err = o.mergeDocument(typedLeft, doc, parentMatchChildDefaults)
			case AnnotationRemove:
				err = o.removeDocument(typedLeft, doc, parentMatchChildDefaults)
			case AnnotationReplace:
				err = o.replaceDocument(typedLeft, doc, parentMatchChildDefaults)
			case AnnotationInsert:
				err = o.insertDocument(typedLeft, doc, parentMatchChildDefaults)
			case AnnotationAppend:
				err = o.appendDocument(typedLeft, doc)
			case AnnotationAssert:
				err = o.assertDocument(typedLeft, doc, parentMatchChildDefaults)
			default:
				err = fmt.Errorf("Overlay op %s is not supported on document", op)
			}
		}
		if err != nil {
			return false, fmt.Errorf("Document on %s: %s", doc.Position.AsString(), err)
		}
	}

	return false, nil
}

func (o Op) removeOverlayAnns(val interface{}) {
	node, ok := val.(yamlmeta.Node)
	if !ok {
		return
	}

	template.NewAnnotations(node).DeleteNs(AnnotationNs)

	for _, childVal := range node.GetValues() {
		o.removeOverlayAnns(childVal)
	}
}
