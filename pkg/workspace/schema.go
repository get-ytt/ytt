// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package workspace

import (
	"github.com/k14s/ytt/pkg/schema"
	"github.com/k14s/ytt/pkg/yamlmeta"
)

type Schema interface {
	AssignType(typeable yamlmeta.Typeable) yamlmeta.TypeCheck
	DefaultDataValues() *yamlmeta.Document
	ValidateWithValues(valuesFilesCount int) error
}

var _ Schema = &schema.AnySchema{}
var _ Schema = &schema.DocumentSchema{}
var _ Schema = &schema.NullSchema{}
