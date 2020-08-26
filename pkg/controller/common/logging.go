// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconciliationLogger returns a logger with labels set to the reconciliation request details.
func ReconciliationLogger(parent logr.Logger, request reconcile.Request, iteration uint64) logr.Logger {
	return parent.WithValues("labels", map[string]interface{}{
		"reconcile.name":      request.Name,
		"reconcile.namespace": request.Namespace,
		"reconcile.iteration": iteration,
	})
}

// LogReconciliationRun is the common logging function used to record a reconciliation run.
func LogReconciliationRun(log logr.Logger, request reconcile.Request) func() {
	startTime := time.Now()

	log.Info("Starting reconciliation run")

	return func() {
		totalTime := time.Since(startTime)
		log.Info("Ending reconciliation run", "took", totalTime)
	}
}
