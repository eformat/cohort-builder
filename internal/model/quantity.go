package model

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

func parseQuantity(s string) error {
	_, err := resource.ParseQuantity(s)
	if err != nil {
		return fmt.Errorf("invalid quantity %q: %v", s, err)
	}
	return nil
}
