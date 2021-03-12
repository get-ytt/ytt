// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package overlay

import (
	"github.com/k14s/ytt/pkg/yamlmeta"
)

func (o Op) mergeArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) error {

	matchChildDefaults, err := NewMatchChildDefaultsAnnotation(newItem, parentMatchChildDefaults)
	if err != nil {
		return err
	}

	ann, err := NewArrayItemMatchAnnotation(newItem, parentMatchChildDefaults, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.Indexes(leftArray)
	if err != nil {
		if err, ok := err.(MatchAnnotationNumMatchError); ok && err.isConditional() {
			return nil
		}
		return err
	}

	if len(leftIdxs) == 0 {
		return o.appendArrayItem(leftArray, newItem)
	}

	for _, leftIdx := range leftIdxs {
		replace := true
		if leftArray.Items[leftIdx].Value != nil {
			replace, err = o.apply(leftArray.Items[leftIdx].Value, newItem.Value, matchChildDefaults)
			if err != nil {
				return err
			}
		}
		if replace {
			leftArray.Items[leftIdx].Value = newItem.Value
			//todo: should we be setting the position elsewhere?
			leftArray.Items[leftIdx].Position = newItem.Position
		}
	}

	return nil
}

func (o Op) removeArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) error {

	ann, err := NewArrayItemMatchAnnotation(newItem, parentMatchChildDefaults, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.Indexes(leftArray)
	if err != nil {
		if err, ok := err.(MatchAnnotationNumMatchError); ok && err.isConditional() {
			return nil
		}
		return err
	}

	for _, leftIdx := range leftIdxs {
		leftArray.Items[leftIdx] = nil
	}

	// Prune out all nil items
	updatedItems := []*yamlmeta.ArrayItem{}

	for _, item := range leftArray.Items {
		if item != nil {
			updatedItems = append(updatedItems, item)
		}
	}

	leftArray.Items = updatedItems

	return nil
}

func (o Op) replaceArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) error {

	ann, err := NewArrayItemMatchAnnotation(newItem, parentMatchChildDefaults, o.Thread)
	if err != nil {
		return err
	}

	replaceAnn, err := NewReplaceAnnotation(newItem, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.Indexes(leftArray)
	if err != nil {
		if err, ok := err.(MatchAnnotationNumMatchError); ok && err.isConditional() {
			return nil
		}
		return err
	}

	for _, leftIdx := range leftIdxs {
		newVal, err := replaceAnn.Value(leftArray.Items[leftIdx])
		if err != nil {
			return err
		}

		leftArray.Items[leftIdx] = newItem.DeepCopy()
		err = leftArray.Items[leftIdx].SetValue(newVal)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o Op) insertArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) error {

	ann, err := NewArrayItemMatchAnnotation(newItem, parentMatchChildDefaults, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.Indexes(leftArray)
	if err != nil {
		if err, ok := err.(MatchAnnotationNumMatchError); ok && err.isConditional() {
			return nil
		}
		return err
	}

	insertAnn, err := NewInsertAnnotation(newItem)
	if err != nil {
		return err
	}

	updatedItems := []*yamlmeta.ArrayItem{}

	for i, leftItem := range leftArray.Items {
		matched := false
		for _, leftIdx := range leftIdxs {
			if i == leftIdx {
				matched = true
				if insertAnn.IsBefore() {
					updatedItems = append(updatedItems, newItem.DeepCopy())
				}
				updatedItems = append(updatedItems, leftItem)
				if insertAnn.IsAfter() {
					updatedItems = append(updatedItems, newItem.DeepCopy())
				}
				break
			}
		}
		if !matched {
			updatedItems = append(updatedItems, leftItem)
		}
	}

	leftArray.Items = updatedItems

	return nil
}

func (o Op) appendArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem) error {

	// No need to traverse further
	leftArray.Items = append(leftArray.Items, newItem.DeepCopy())
	return nil
}

func (o Op) assertArrayItem(
	leftArray *yamlmeta.Array, newItem *yamlmeta.ArrayItem,
	parentMatchChildDefaults MatchChildDefaultsAnnotation) error {

	matchChildDefaults, err := NewMatchChildDefaultsAnnotation(newItem, parentMatchChildDefaults)
	if err != nil {
		return err
	}

	ann, err := NewArrayItemMatchAnnotation(newItem, parentMatchChildDefaults, o.Thread)
	if err != nil {
		return err
	}

	testAnn, err := NewAssertAnnotation(newItem, o.Thread)
	if err != nil {
		return err
	}

	leftIdxs, err := ann.Indexes(leftArray)
	if err != nil {
		if err, ok := err.(MatchAnnotationNumMatchError); ok && err.isConditional() {
			return nil
		}
		return err
	}

	for _, leftIdx := range leftIdxs {
		err := testAnn.Check(leftArray.Items[leftIdx])
		if err != nil {
			return err
		}

		_, err = o.apply(leftArray.Items[leftIdx].Value, newItem.Value, matchChildDefaults)
		if err != nil {
			return err
		}
	}

	return nil
}
