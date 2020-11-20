// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    *JobDef
		wantErr bool
	}{
		{
			name:  "valid YAML",
			input: "testdata/valid_yaml.yaml",
			want: &JobDef{
				NoProgressTimeout: Duration(1500 * time.Second),
				Target: ResourceRef{
					Kind:      ResourceKindElasticsearch,
					Namespace: "default",
					Name:      "quickstart",
				},
				ClientConf: &ClientConf{
					RequestTimeout:   Duration(30 * time.Second),
					RetryAttempts:    3,
					RetryBackoff:     Duration(10 * time.Second),
					RetryMaxDuration: Duration(300 * time.Second),
				},
				APICalls: []APICall{
					{
						Method:       MethodPost,
						Path:         "_ilm/policy/my_policy",
						Payload:      json.RawMessage(`{"k":"v"}`),
						SuccessCodes: []int{200},
						Retry:        true,
					},
				},
			},
		},
		{
			name:  "valid JSON",
			input: "testdata/valid_json.json",
			want: &JobDef{
				NoProgressTimeout: Duration(1500 * time.Second),
				Target: ResourceRef{
					Kind:      ResourceKindElasticsearch,
					Namespace: "default",
					Name:      "quickstart",
				},
				ClientConf: &ClientConf{
					RequestTimeout:   Duration(30 * time.Second),
					RetryAttempts:    3,
					RetryBackoff:     Duration(10 * time.Second),
					RetryMaxDuration: Duration(300 * time.Second),
				},
				APICalls: []APICall{
					{
						Method:       MethodPost,
						Path:         "_ilm/policy/my_policy",
						Payload:      json.RawMessage(`{"k":"v"}`),
						SuccessCodes: []int{200},
						Retry:        true,
					},
				},
			},
		},
		{
			name:    "bad kind",
			input:   "testdata/bad_kind.yaml",
			wantErr: true,
		},
		{
			name:    "invalid target",
			input:   "testdata/invalid_target.yaml",
			wantErr: true,
		},
		{
			name:    "invalid API call",
			input:   "testdata/invalid_api_call.yaml",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open(tc.input)
			require.NoError(t, err)

			defer f.Close()

			jd, err := Load(f)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, jd)
		})
	}
}
