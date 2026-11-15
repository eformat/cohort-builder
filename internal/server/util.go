package server

import (
	"bufio"
	"bytes"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// splitYAML splits a multi-document YAML string into individual documents.
func splitYAML(multi string) []string {
	var docs []string
	r := yaml.NewYAMLReader(bufio.NewReader(strings.NewReader(multi)))
	for {
		doc, err := r.Read()
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, string(doc))
		}
		if err != nil {
			break
		}
	}
	return docs
}
