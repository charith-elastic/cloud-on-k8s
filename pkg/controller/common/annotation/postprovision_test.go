// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPostProvisionComplete(t *testing.T) {
	testCases := []struct {
		name    string
		objMeta metav1.ObjectMeta
		want    bool
	}{
		{
			name:    "empty",
			objMeta: metav1.ObjectMeta{},
			want:    false,
		},
		{
			name:    "not ok",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{PostProvisionCompleteAnnotation: "false"}},
			want:    false,
		},
		{
			name:    "ok",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{PostProvisionCompleteAnnotation: "true"}},
			want:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := IsPostProvisionComplete(tc.objMeta)
			require.Equal(t, tc.want, have)
		})
	}
}

func TestSetPostProvisionComplete(t *testing.T) {
	testCases := []struct {
		name    string
		obj     *esv1.Elasticsearch
		want    *esv1.Elasticsearch
		wantErr bool
	}{
		{
			name: "valid object",
			obj:  &esv1.Elasticsearch{},
			want: &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PostProvisionCompleteAnnotation: "true"}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := SetPostProvisionComplete(tc.obj)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, tc.obj)
		})
	}
}
