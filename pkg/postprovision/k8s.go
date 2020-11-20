// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"context"
	"fmt"

	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func newK8sClient() (client.Client, error) {
	controllerscheme.SetupScheme()

	conf, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	return client.New(conf, client.Options{})
}

func watchObject(ctx context.Context, namespace string, handler toolscache.ResourceEventHandler, obj runtime.Object) error {
	controllerscheme.SetupScheme()

	conf, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	c, err := cache.New(conf, cache.Options{Namespace: namespace})
	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	informer, err := c.GetInformer(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to get informer: %w", err)
	}

	informer.AddEventHandler(handler)

	go c.Start(ctx.Done())

	return nil
}
