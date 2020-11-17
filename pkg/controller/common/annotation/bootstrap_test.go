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

func TestGetBootstrapReadinessGate(t *testing.T) {
	testCases := []struct {
		name    string
		objMeta metav1.ObjectMeta
		want    string
	}{
		{
			name:    "empty",
			objMeta: metav1.ObjectMeta{},
			want:    "",
		},
		{
			name:    "set",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{BootstrapReadinessGateAnnotation: "mygate"}},
			want:    "mygate",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := GetBootstrapReadinessGate(tc.objMeta)
			require.Equal(t, tc.want, have)
		})
	}
}

func TestIsBootstrapped(t *testing.T) {
	testCases := []struct {
		name    string
		objMeta metav1.ObjectMeta
		want    bool
	}{
		{
			name:    "empty",
			objMeta: metav1.ObjectMeta{},
			want:    true,
		},
		{
			name:    "no readiness gate",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{BootstrappedAnnotation: "rubbish"}},
			want:    true,
		},
		{
			name:    "not ok",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{BootstrapReadinessGateAnnotation: "mygate", BootstrappedAnnotation: "false"}},
			want:    false,
		},
		{
			name:    "ok",
			objMeta: metav1.ObjectMeta{Annotations: map[string]string{BootstrapReadinessGateAnnotation: "mygate", BootstrappedAnnotation: "true"}},
			want:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := IsBootstrapped(tc.objMeta)
			require.Equal(t, tc.want, have)
		})
	}
}

func TestSetBootstrapped(t *testing.T) {
	testCases := []struct {
		name    string
		obj     *esv1.Elasticsearch
		want    *esv1.Elasticsearch
		wantErr bool
	}{
		{
			name: "valid object",
			obj:  &esv1.Elasticsearch{},
			want: &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{BootstrappedAnnotation: "true"}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := SetBootstrapped(tc.obj)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, tc.obj)
		})
	}
}
