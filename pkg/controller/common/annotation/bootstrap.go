// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	BootstrappedAnnotation           = "eck.k8s.elastic.co/bootstrapped"
	BootstrapReadinessGateAnnotation = "eck.k8s.elastic.co/bootstrap-readiness-gate"
)

// GetBootstrapReadinessGate returns the name of the readiness gate specified in the annotation.
func GetBootstrapReadinessGate(objMeta metav1.ObjectMeta) string {
	if len(objMeta.Annotations) == 0 {
		return ""
	}

	return objMeta.Annotations[BootstrapReadinessGateAnnotation]
}

// IsBootstrapped returns true if the object has the bootstrapped annotation.
func IsBootstrapped(objMeta metav1.ObjectMeta) bool {
	if GetBootstrapReadinessGate(objMeta) == "" {
		return true
	}

	return objMeta.Annotations[BootstrappedAnnotation] == "true"
}

// SetBootstrapped sets the bootstrapped annotation on the object.
func SetBootstrapped(obj runtime.Object) error {
	if obj == nil {
		return nil
	}

	accessor := meta.NewAccessor()

	annotations, err := accessor.Annotations(obj)
	if err != nil {
		return err
	}

	if annotations == nil {
		annotations = make(map[string]string, 1)
	}

	annotations[BootstrappedAnnotation] = "true"

	return accessor.SetAnnotations(obj, annotations)
}
