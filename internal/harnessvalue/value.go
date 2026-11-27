// Package harnessvalue delegates bounded, immutable harness policy values to
// github.com/relux-works/archon-pantheon/pkg/harnessvalue.
package harnessvalue

import upstream "github.com/relux-works/archon-pantheon/pkg/harnessvalue"

const (
	MaxTextBytes = upstream.MaxTextBytes
	MaxPathBytes = upstream.MaxPathBytes
	MaxItems     = upstream.MaxItems
)

type RelativePath = upstream.RelativePath
type Bytes = upstream.Bytes

func NewRelativePath(value string) (RelativePath, error) { return upstream.NewRelativePath(value) }
func NewBytes(value []byte) (Bytes, error)               { return upstream.NewBytes(value) }
func Text(value string) error                            { return upstream.Text(value) }
