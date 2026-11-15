package validate

import (
	"bufio"
	"bytes"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// splitDocs splits a multi-document YAML string into individual documents.
func splitDocs(multi string) []string {
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

func sigsyamlToJSON(doc string) ([]byte, error) {
	return sigsyaml.YAMLToJSON([]byte(doc))
}
