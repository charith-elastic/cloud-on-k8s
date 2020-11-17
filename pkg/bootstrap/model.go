// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var errInvalidJobDef = errors.New("invalid job definition")

// JobDef represents the structure of a job definition.
type JobDef struct {
	Target     ResourceRef `json:"target"`
	APICalls   []APICall   `json:"apiCalls"`
	ClientConf *ClientConf `json:"clientConf"`
}

// ResourceRef defines a reference to an ECK resource.
type ResourceRef struct {
	Kind      ResourceKind `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
}

// ResourceKind defines the kind of a resource.
type ResourceKind string

const (
	ResourceKindElasticsearch ResourceKind = "Elasticsearch"
)

func (rk *ResourceKind) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	tmp := ResourceKind(v)
	if tmp != ResourceKindElasticsearch {
		return fmt.Errorf("unknown resource kind: %s", v)
	}

	*rk = tmp

	return nil
}

// ClientConf defines common settings for API calls.
type ClientConf struct {
	RequestTimeout   Duration `json:"requestTimeout"`
	RetryAttempts    uint8    `json:"retryAttempts"`
	RetryBackoff     Duration `json:"retryBackoff"`
	RetryMaxDuration Duration `json:"retryMaxDuration"`
}

// ToBackoff creates a Backoff object from the config.
func (cc *ClientConf) ToBackoff() wait.Backoff {
	if cc == nil {
		return wait.Backoff{
			Steps: 1,
		}
	}

	return wait.Backoff{
		Duration: time.Duration(cc.RetryBackoff),
		Factor:   2,
		Jitter:   0.5,
		Steps:    int(cc.RetryAttempts),
		Cap:      time.Duration(cc.RetryMaxDuration),
	}
}

// Duration is an alias for time.Duration
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	duration, err := time.ParseDuration(v)
	if err != nil {
		return err
	}

	*d = Duration(duration)

	return nil
}

// APICall defines the structure of an API call.
type APICall struct {
	Method       APIMethod       `json:"method"`
	Path         string          `json:"path"`
	Payload      json.RawMessage `json:"payload"`
	SuccessCodes []int           `json:"successCodes"`
	Retry        bool
}

// IsSuccessful returns true if the given code is one of the success codes.
func (ac APICall) IsSuccessful(code int) bool {
	for _, c := range ac.SuccessCodes {
		if code == c {
			return true
		}
	}

	return false
}

// APIMethod defines the allowed API methods.
type APIMethod string

const (
	MethodGet    APIMethod = http.MethodGet
	MethodHead   APIMethod = http.MethodHead
	MethodPost   APIMethod = http.MethodPost
	MethodPut    APIMethod = http.MethodPut
	MethodPatch  APIMethod = http.MethodPatch
	MethodDelete APIMethod = http.MethodDelete
)

func (am *APIMethod) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	tmp := APIMethod(v)
	switch tmp {
	case MethodGet:
	case MethodHead:
	case MethodPost:
	case MethodPut:
	case MethodPatch:
	case MethodDelete:
	default:
		return fmt.Errorf("unknown method: %s", v)
	}

	*am = tmp

	return nil
}

// Load attempts to deserialize a job definition from the provided reader.
func Load(r io.Reader) (*JobDef, error) {
	d := yaml.NewYAMLOrJSONDecoder(r, 64)

	jobDef := new(JobDef)
	if err := d.Decode(jobDef); err != nil {
		return nil, fmt.Errorf("failed to decode job definition: %w", err)
	}

	if err := validate(jobDef); err != nil {
		return nil, err
	}

	return jobDef, nil
}

func validate(jd *JobDef) error {
	var errDesc []string

	if isEmpty(jd.Target.Name) {
		errDesc = append(errDesc, "Target name is required")
	}

	if isEmpty(jd.Target.Namespace) {
		errDesc = append(errDesc, "Target namespace is required")
	}

	for i, ac := range jd.APICalls {
		if isEmpty(ac.Path) {
			errDesc = append(errDesc, fmt.Sprintf("API call %d is missing the path field", i))
		}
	}

	if len(errDesc) > 0 {
		return fmt.Errorf("invalid job definition [%s]: %w", strings.Join(errDesc, ","), errInvalidJobDef)
	}

	return nil
}

func isEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}
