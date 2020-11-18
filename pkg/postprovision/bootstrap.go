// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Run starts the execution of the job.
func Run(ctx context.Context, jd *JobDef) error {
	if jd.Target.Kind != ResourceKindElasticsearch {
		return fmt.Errorf("unhandled resource type %s", jd.Target.Kind)
	}

	c, err := newK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return runElasticsearchJob(ctx, c, jd)
}

func annotateAsDone(ctx context.Context, c client.Client, key client.ObjectKey, obj runtime.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, key, obj); err != nil {
			return err
		}

		if err := annotation.SetPostProvisionComplete(obj); err != nil {
			return err
		}

		if err := c.Update(ctx, obj); err != nil {
			return err
		}

		return nil
	})
}
