// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "postprovision"

var log = logf.Log.WithName(controllerName)

// Add registers the controller with the runtime.
func Add(mgr manager.Manager, p operator.Parameters) error {
	r := newReconciler(mgr, p)
	c, err := common.NewController(mgr, controllerName, r, p)
	if err != nil {
		return err
	}

	return addWatches(c, r.client)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *reconcilePostProvision {
	c := k8s.WrapClient(mgr.GetClient())
	return &reconcilePostProvision{client: c}
}

func addWatches(ctrlr controller.Controller, c k8s.Client) error {
	// Watch pods belonging to ES clusters that have a pod condition
	return ctrlr.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(
			func(object handler.MapObject) []reconcile.Request {
				labels := object.Meta.GetLabels()

				if len(labels) == 0 {
					return nil
				}

				// is this a pod belonging to an Elasticsearch cluster?
				_, ok := labels[label.ClusterNameLabelName]
				if !ok {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: object.Meta.GetNamespace(),
							Name:      object.Meta.GetName(),
						},
					},
				}
			})})
}

type reconcilePostProvision struct {
	client    k8s.Client
	iteration uint64
}

func (rpp *reconcilePostProvision) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "es_name", &rpp.iteration)()

	result := reconcile.Result{}

	var pod corev1.Pod
	if err := rpp.client.Get(request.NamespacedName, &pod); err != nil {
		return result, err
	}

	labels := pod.GetLabels()
	if len(labels) == 0 {
		return result, nil
	}

	esName, ok := labels[label.ClusterNameLabelName]
	if !ok {
		return result, nil
	}

	var es esv1.Elasticsearch
	if err := rpp.client.Get(client.ObjectKey{Namespace: pod.Namespace, Name: esName}, &es); err != nil {
		return result, err
	}

	rg := annotation.GetPostProvisionReadinessGate(es.ObjectMeta)
	if rg == "" {
		return result, nil
	}

	// requeue if post provision has not completed
	if !annotation.IsPostProvisionComplete(es.ObjectMeta) {
		return reconcile.Result{Requeue: true}, nil
	}

	for i, c := range pod.Status.Conditions {
		if c.Type == corev1.PodConditionType(rg) && c.Status != corev1.ConditionTrue {
			pod.Status.Conditions[i].Status = corev1.ConditionTrue
			return result, rpp.client.Status().Update(&pod)
		}
	}

	return result, nil
}
