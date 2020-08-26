// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"sync/atomic"

	"github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewReconciliationContext creates the context for a reconciliation run.
// It increments the iteration number and embeds a logger into the returned context.
func NewReconciliationContext(request reconcile.Request, iteration *uint64, logger logr.Logger) context.Context {
	currIteration := atomic.AddUint64(iteration, 1)
	rlog := ReconciliationLogger(logger, request, currIteration)

	return log.IntoContext(context.Background(), rlog)
}
