// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultTimeout = 15 * time.Minute

var (
	errNoAvailablePods = errors.New("no available Elasticsearch pods")
	errResourceDeleted = errors.New("resource deleted")
	errRetry           = errors.New("retry")
)

func runElasticsearchJob(ctx context.Context, k8sclient client.Client, jd *JobDef) error {
	log := logf.FromContext(ctx)

	log.V(1).Info("Waiting for Elasticsearch resource")
	es, err := waitForElasticsearch(ctx, k8sclient, jd)
	if err != nil {
		log.Error(err, "Failed to find Elasticsearch")
		return fmt.Errorf("failed to find Elasticsearch: %w", err)
	}

	log.V(1).Info("Creating Elasticsearch client")
	c, err := getElasticsearchClient(ctx, k8sclient, jd, es)
	if err != nil {
		log.Error(err, "Failed to create Elasticsearch client")
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	defer c.Close()

	log.V(1).Info("Issuing API calls")
	if err := issueAPICalls(ctx, jd, c); err != nil {
		log.Error(err, "Failed to issue API calls")
		return fmt.Errorf("failed to issue API calls: %w", err)
	}

	if err := annotateAsDone(ctx, k8sclient, k8s.ExtractNamespacedName(es), &esv1.Elasticsearch{}); err != nil {
		log.Error(err, "Failed to annotate Elasticsearch")
		return fmt.Errorf("failed to annotate Elasticsearch: %w", err)
	}

	return nil
}

type esHolder struct {
	es  *esv1.Elasticsearch
	err error
}

func waitForElasticsearch(ctx context.Context, k8sclient client.Client, jd *JobDef) (*esv1.Elasticsearch, error) {
	log := logf.FromContext(ctx)

	result := make(chan esHolder, 1)
	defer close(result)

	checkESReady := func(obj interface{}) {
		es, ok := obj.(*esv1.Elasticsearch)
		if !ok {
			return
		}

		if es.Name != jd.Target.Name {
			return
		}

		c, err := getElasticsearchClient(ctx, k8sclient, jd, es)
		if err != nil {
			log.V(1).Info("Failed to get Elasticsearch client", "error", err)
			return
		}

		h, err := c.GetClusterHealth(ctx)
		if err != nil {
			log.V(1).Info("Failed to get Elasticsearch health", "error", err)
			return
		}

		log.V(1).Info("Elasticsearch health", "health", h)

		if h.Status == esv1.ElasticsearchGreenHealth {
			result <- esHolder{es: es}
		}
	}

	handler := toolscache.ResourceEventHandlerFuncs{
		AddFunc: checkESReady,
		UpdateFunc: func(oldObj, newObj interface{}) {
			checkESReady(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			es, ok := obj.(*esv1.Elasticsearch)
			if !ok {
				return
			}

			if es.Name != jd.Target.Name {
				return
			}

			result <- esHolder{err: errResourceDeleted}
		},
	}

	timeout := defaultTimeout
	if jd.NoProgressTimeout > 0 {
		timeout = time.Duration(jd.NoProgressTimeout)
	}

	ctx, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()

	if err := watchObject(ctx, jd.Target.Namespace, handler, &esv1.Elasticsearch{}); err != nil {
		return nil, fmt.Errorf("failed to watch object: %w", err)
	}

	r := <-result

	return r.es, r.err
}

func getElasticsearchClient(ctx context.Context, k8sclient client.Client, jd *JobDef, es *esv1.Elasticsearch) (esclient.Client, error) {
	url, err := getElasticsearchURL(ctx, k8sclient, jd, es)
	if err != nil {
		return nil, err
	}

	var requestTimeout time.Duration
	if jd.ClientConf != nil && jd.ClientConf.RequestTimeout > 0 {
		requestTimeout = time.Duration(jd.ClientConf.RequestTimeout)
	} else {
		requestTimeout = esclient.Timeout(*es)
	}

	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Elasticsearch version: %w", err)
	}

	log := logf.FromContext(ctx)
	log.V(1).Info(fmt.Sprintf("Elasticsearch connection settings: VERSION=%s URL=%s REQTIMEOUT=%s", ver.String(), url, requestTimeout.String()))

	var certs []*x509.Certificate
	if es.Spec.HTTP.TLS.Enabled() {
		secretRef := certificates.PublicCertsSecretRef(esv1.ESNamer, k8s.ExtractNamespacedName(es))

		var certSecret corev1.Secret
		if err := k8sclient.Get(ctx, secretRef, &certSecret); err != nil {
			return nil, fmt.Errorf("failed to get public certs secret %s: %w", secretRef.String(), err)
		}

		certs, err = certificates.ParsePEMCerts(certSecret.Data[certificates.CertFileName])
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificates: %w", err)
		}
	}

	var userSecret corev1.Secret
	if err := k8sclient.Get(ctx, client.ObjectKey{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)}, &userSecret); err != nil {
		return nil, fmt.Errorf("failed to get user secret: %w", err)
	}

	auth := esclient.BasicAuth{Name: user.ElasticUserName, Password: string(userSecret.Data[user.ElasticUserName])}

	return esclient.NewElasticsearchClient(nil, url, auth, *ver, certs, requestTimeout), nil
}

func getElasticsearchURL(ctx context.Context, k8sclient client.Client, jd *JobDef, es *esv1.Elasticsearch) (string, error) {
	// If there's no readiness gate, the service can be accessed directly.
	if !hasReadinessGate(es) {
		return services.ExternalServiceURL(*es), nil
	}

	labelSelector := label.NewLabelSelectorForElasticsearch(*es)

	var podList corev1.PodList
	if err := k8sclient.List(ctx, &podList, client.InNamespace(es.Namespace), labelSelector); err != nil {
		return "", fmt.Errorf("failed to list Elasticsearch pods: %w", err)
	}

	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning && p.Status.PodIP != "" {
			protocol := es.Spec.HTTP.Protocol()
			return fmt.Sprintf("%s://%s:%d", protocol, p.Status.PodIP, network.HTTPPort), nil
		}
	}

	return "", errNoAvailablePods
}

func hasReadinessGate(es *esv1.Elasticsearch) bool {
	for _, ns := range es.Spec.NodeSets {
		for _, rg := range ns.PodTemplate.Spec.ReadinessGates {
			if rg.ConditionType == ReadinessGate {
				return true
			}
		}
	}

	return false
}

func issueAPICalls(ctx context.Context, jd *JobDef, c esclient.Client) error {
	logger := logf.FromContext(ctx)

	for i, ac := range jd.APICalls {
		log := logger.WithValues("call_seq", i)

		req, err := toESRequest(ac)
		if err != nil {
			log.Error(err, "Failed to construct request")
			return fmt.Errorf("failed to construct request %d: %w", i, err)
		}

		backoff := jd.ClientConf.ToBackoff()

		if err := retry.OnError(
			backoff,
			func(err error) bool { return errors.Is(err, errRetry) },
			func() error { return makeESRequest(ctx, log, c, ac, req) }); err != nil {
			log.Error(err, "Aborting due to API call failure")
			return err
		}
	}

	return nil
}

func makeESRequest(ctx context.Context, log logr.Logger, c esclient.Client, ac APICall, req *http.Request) error {
	log.V(1).Info("Sending request")

	resp, err := c.Request(ctx, req)
	if err != nil {
		log.Error(err, "Request failed")
		if ac.Retry {
			return errRetry
		}

		return fmt.Errorf("request failed: %w", err)
	}

	defer func() {
		if resp.Body != nil {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	if log.V(1).Enabled() {
		if respBody, err := httputil.DumpResponse(resp, true); err == nil {
			log.V(1).Info("Received response", "body", string(respBody))
		}
	}

	if ac.IsSuccessful(resp.StatusCode) {
		log.Info("Request successful", "status_code", resp.StatusCode)
		return nil
	}

	err = fmt.Errorf("request failed with status code %d", resp.StatusCode)
	log.Error(err, "Request failed", "status_code", resp.StatusCode)

	if ac.Retry {
		return errRetry
	}

	return err
}

func toESRequest(ac APICall) (*http.Request, error) {
	var body io.Reader
	if len(ac.Payload) > 0 {
		body = bytes.NewReader([]byte(ac.Payload))
	}

	path := ac.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return http.NewRequest(string(ac.Method), path, body)
}
