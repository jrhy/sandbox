package gob2json

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
)

type MastNode struct {
	Key   []interface{}
	Value []interface{}
	Link  []interface{}
}

type MastRoot struct {
	Link         string
	Size         uint64
	Height       uint8
	BranchFactor uint
	NodeFormat   string `json:"NodeFormat,omitempty"`
}

type CRDTRoot struct {
	MergeSources []string
	Source       string
	Root         MastRoot
}

func Read(b []byte, i interface{}) (string, error) {
	decoder := gob.NewDecoder(bytes.NewReader(b))
	err := decoder.Decode(i)
	if err != nil {
		return "", fmt.Errorf("decode gob: %w", err)
	}
	s, err := json.Marshal(i)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(s), nil
}

func ReadRoot(b []byte) (string, error) {
	return Read(b, &CRDTRoot{})
}
