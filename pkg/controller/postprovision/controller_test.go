// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package postprovision

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/postprovision"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	t.Run("post_provision_complete", func(t *testing.T) {
		t.Run("pod_without_status", func(t *testing.T) {
			c := k8s.WrappedFakeClient(mkElasticsearch(true), mkPod(nil))
			r := &reconcilePostProvision{client: c}

			result, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-es-ns1-0", Namespace: "testns"}})
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, result)

			var pod corev1.Pod
			require.NoError(t, c.Get(client.ObjectKey{Name: "test-es-ns1-0", Namespace: "testns"}, &pod))
			require.Len(t, pod.Status.Conditions, 1)
			require.Equal(t, corev1.PodConditionType(postprovision.ReadinessGate), pod.Status.Conditions[0].Type)
			require.Equal(t, corev1.ConditionTrue, pod.Status.Conditions[0].Status)
		})

		t.Run("pod_with_status", func(t *testing.T) {
			st := corev1.ConditionFalse
			c := k8s.WrappedFakeClient(mkElasticsearch(true), mkPod(&st))
			r := &reconcilePostProvision{client: c}

			result, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-es-ns1-0", Namespace: "testns"}})
			require.NoError(t, err)
			require.Equal(t, reconcile.Result{}, result)

			var pod corev1.Pod
			require.NoError(t, c.Get(client.ObjectKey{Name: "test-es-ns1-0", Namespace: "testns"}, &pod))
			require.Len(t, pod.Status.Conditions, 1)
			require.Equal(t, corev1.PodConditionType(postprovision.ReadinessGate), pod.Status.Conditions[0].Type)
			require.Equal(t, corev1.ConditionTrue, pod.Status.Conditions[0].Status)
		})
	})

	t.Run("post_provision_not_complete", func(t *testing.T) {
		c := k8s.WrappedFakeClient(mkElasticsearch(false), mkPod(nil))
		r := &reconcilePostProvision{client: c}

		result, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-es-ns1-0", Namespace: "testns"}})
		require.NoError(t, err)
		require.Equal(t, reconcile.Result{}, result)

		var pod corev1.Pod
		require.NoError(t, c.Get(client.ObjectKey{Name: "test-es-ns1-0", Namespace: "testns"}, &pod))
		require.Len(t, pod.Status.Conditions, 1)
		require.Equal(t, corev1.PodConditionType(postprovision.ReadinessGate), pod.Status.Conditions[0].Type)
		require.Equal(t, corev1.ConditionFalse, pod.Status.Conditions[0].Status)
	})
}

func mkElasticsearch(annotate bool) *esv1.Elasticsearch {
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testns",
		},
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:  "ns1",
					Count: 3,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ReadinessGates: []corev1.PodReadinessGate{{ConditionType: postprovision.ReadinessGate}},
						},
					},
				},
				{
					Name:  "ns2",
					Count: 3,
				},
			},
		},
	}

	if annotate {
		es.ObjectMeta.Annotations = map[string]string{annotation.PostProvisionCompleteAnnotation: "true"}
	}

	return es
}

func mkPod(status *corev1.ConditionStatus) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es-ns1-0",
			Namespace: "testns",
			Labels: map[string]string{
				label.ClusterNameLabelName:     "test",
				label.StatefulSetNameLabelName: "test-es-ns1",
			},
		},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{{ConditionType: postprovision.ReadinessGate}},
		},
	}

	if status == nil {
		return pod
	}

	pod.Status = corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{Type: postprovision.ReadinessGate, Status: *status},
		},
	}

	return pod
}
