// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/postprovision"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return &reconcilePostProvision{Parameters: params, client: c}
}

func addWatches(ctrlr controller.Controller, c k8s.Client) error {
	// watch Elasticsearch clusters
	err := ctrlr.Watch(
		&source.Kind{Type: &esv1.Elasticsearch{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(object handler.MapObject) []reconcile.Request {
				es, ok := object.Object.(*esv1.Elasticsearch)
				if !ok {
					return nil
				}

				// find pods that have the readiness gate defined
				var requests []reconcile.Request
				for _, ns := range es.Spec.NodeSets {
					for _, rg := range ns.PodTemplate.Spec.ReadinessGates {
						if rg.ConditionType == postprovision.ReadinessGate {
							sts := esv1.StatefulSet(es.Name, ns.Name)
							selector := label.NewStatefulSetLabels(k8s.ExtractNamespacedName(es), sts)

							var pods corev1.PodList
							if err := c.List(&pods, client.MatchingLabels(selector)); err != nil {
								return nil
							}

							for _, p := range pods.Items {
								requests = append(requests, reconcile.Request{
									NamespacedName: types.NamespacedName{
										Namespace: p.GetNamespace(),
										Name:      p.GetName(),
									},
								})
							}
						}
					}
				}

				return requests
			}),
		})
	if err != nil {
		return err
	}

	// Watch pods belonging to ES clusters that have a readiness gate
	return ctrlr.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(object handler.MapObject) []reconcile.Request {
				labels := object.Meta.GetLabels()

				if len(labels) == 0 {
					return nil
				}

				// is this a pod belonging to an Elasticsearch cluster?
				_, ok := labels[label.ClusterNameLabelName]
				if !ok {
					return nil
				}

				pod, ok := object.Object.(*corev1.Pod)
				if !ok {
					return nil
				}

				for _, rg := range pod.Spec.ReadinessGates {
					if rg.ConditionType == postprovision.ReadinessGate {
						return []reconcile.Request{
							{
								NamespacedName: types.NamespacedName{
									Namespace: object.Meta.GetNamespace(),
									Name:      object.Meta.GetName(),
								},
							},
						}
					}
				}

				return nil
			}),
		})
}

type reconcilePostProvision struct {
	operator.Parameters
	client    k8s.Client
	iteration uint64
}

func (rpp *reconcilePostProvision) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "es_name", &rpp.iteration)()
	tx, ctx := tracing.NewTransaction(rpp.Tracer, request.NamespacedName, "postprovision")
	defer tracing.EndTransaction(tx)

	c := rpp.client.WithContext(ctx)
	result := reconcile.Result{}

	var pod corev1.Pod
	if err := c.Get(request.NamespacedName, &pod); err != nil {
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
	if err := c.Get(client.ObjectKey{Namespace: pod.Namespace, Name: esName}, &es); err != nil {
		if apierrors.IsNotFound(err) {
			return result, nil
		}

		return result, err
	}

	condValue := corev1.ConditionTrue
	if !annotation.IsPostProvisionComplete(es.ObjectMeta) {
		condValue = corev1.ConditionFalse
	}

	now := metav1.NewTime(time.Now())

	found := false
	for i, c := range pod.Status.Conditions {
		if c.Type == postprovision.ReadinessGate {
			found = true

			if c.Status != condValue {
				pod.Status.Conditions[i].Status = condValue
				pod.Status.Conditions[i].LastTransitionTime = now
			}

			pod.Status.Conditions[i].LastProbeTime = now
		}
	}

	if !found {
		pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
			Type:               postprovision.ReadinessGate,
			Status:             condValue,
			LastProbeTime:      now,
			LastTransitionTime: now,
		})
	}

	if err := c.Status().Update(&pod); err != nil {
		if apierrors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}

		return result, err
	}

	return result, nil
}
