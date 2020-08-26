// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	serviceName = "elastic-operator"

	SpanTypeApp = "app"
)

var tracer *apm.Tracer

// InitTracer initializes the global tracer for the application.
func InitTracer() error {
	build := about.GetBuildInfo()

	t, err := apm.NewTracer(serviceName, build.VersionString())
	if err != nil {
		return fmt.Errorf("failed to initialize tracer: %w", err)
	}

	tracer = t

	return nil
}

// Tracer returns the currently configured tracer.
func Tracer() *apm.Tracer {
	return tracer
}

// SetLogger sets the logger for the tracer.
func SetLogger(log logr.Logger) {
	if tracer != nil {
		tracer.SetLogger(NewLogAdapter(log))
	}
}

// CaptureError wraps APM agent func of the same name and auto-sends, returning the original error.
func CaptureError(ctx context.Context, err error) error {
	if ctx != nil {
		apm.CaptureError(ctx, err).Send()
	}

	return err // dropping the apm wrapper here
}

// ReconcilliationFn describes a reconciliation function.
type ReconcilliationFn func(context.Context, reconcile.Request) (reconcile.Result, error)

// TraceReconciliation instruments a reconciliation function for tracing.
func TraceReconciliation(ctx context.Context, request reconcile.Request, kind string, fn ReconcilliationFn) (reconcile.Result, error) {
	t := Tracer()
	if t == nil {
		return fn(ctx, request)
	}

	n := request.NamespacedName.String()

	tx := t.StartTransaction(n, kind)
	defer tx.End()

	newCtx := apm.ContextWithTransaction(ctx, tx)
	result, err := fn(newCtx, request)

	return result, CaptureError(newCtx, err)
}

// DoInSpan wraps the given function within a tracing span.
func DoInSpan(ctx context.Context, name string, fn func(context.Context) error) error {
	span, ctx := apm.StartSpan(ctx, name, SpanTypeApp)
	defer span.End()

	return CaptureError(ctx, fn(ctx))
}
