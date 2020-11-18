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
	PostProvisionCompleteAnnotation      = "eck.k8s.elastic.co/post-provision-complete"
	PostProvisionReadinessGateAnnotation = "eck.k8s.elastic.co/post-provision-readiness-gate"
)

// GetPostProvisionReadinessGate returns the name of the readiness gate specified in the annotation.
func GetPostProvisionReadinessGate(objMeta metav1.ObjectMeta) string {
	if len(objMeta.Annotations) == 0 {
		return ""
	}

	return objMeta.Annotations[PostProvisionReadinessGateAnnotation]
}

// IsPostProvisionComplete returns true if the object has the annotation to indicate that post-provision is complete.
func IsPostProvisionComplete(objMeta metav1.ObjectMeta) bool {
	if GetPostProvisionReadinessGate(objMeta) == "" {
		return true
	}

	return objMeta.Annotations[PostProvisionCompleteAnnotation] == "true"
}

// SetPostProvisionComplete sets the annotation to indicate that post-provision is done.
func SetPostProvisionComplete(obj runtime.Object) error {
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

	annotations[PostProvisionCompleteAnnotation] = "true"

	return accessor.SetAnnotations(obj, annotations)
}
