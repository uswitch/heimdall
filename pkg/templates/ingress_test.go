package templates

import (
	"testing"

	log "github.com/uswitch/heimdall/pkg/log"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	testPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testPod",
			Namespace: "testNamespace",
			Labels: map[string]string{
				"app": "testApp",
			},
			OwnerReferences: []metav1.OwnerReference{
				metav1.OwnerReference{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "testReplicaSet",
				},
			},
		},
	}

	testReplicaset = &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicaSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testReplicaSet",
			Namespace: "testNamespace",
			OwnerReferences: []metav1.OwnerReference{
				metav1.OwnerReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "testApp",
				},
			},
		},
	}

	testService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testService",
			Namespace:   "testNamespace",
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{},
			Selector: map[string]string{
				"app": "testApp",
			},
		},
	}

	testIngressDefaultBackend = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testDefaultBackend",
			Namespace: "testNamespace",
			Annotations: map[string]string{
				"com.uswitch.heimdall/5xx-rate": "0.001",
				ownerAnnotation:                 "testIngressOwner",
				environmentAnnotation:           "testing",
				criticalityAnnotation:           "low",
				sensitivityAnnotation:           "public",
			},
		},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "testService",
					Port: networkingv1.ServiceBackendPort{
						Number: 80,
					},
				},
			},
		},
	}

	testIngressRuleBackend = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testRuleBackend",
			Namespace:   "testNamespace",
			Annotations: map[string]string{"com.uswitch.heimdall/5xx-rate": "0.001"},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				networkingv1.IngressRule{
					Host: "test",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "testService",
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

func TestIngressAnnotationsDefaultBackend(t *testing.T) {
	log.Setup(log.DEBUG_LEVEL)

	client := fake.NewSimpleClientset(testService, testDeployment, testReplicaset, testPod)

	template, err := NewPrometheusRuleTemplateManager("../../kube/config/templates", client)

	expr := `(
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testNamespace",ingress="testDefaultBackend",status=~"5.."}[30s]
    )
  )
  /
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testNamespace",ingress="testDefaultBackend"}[30s]
    )
  )
) > 0.001
`
	promrules, err := template.CreateFromIngress(testIngressDefaultBackend)
	assert.Assert(t, is.Nil(err))
	assert.Assert(t, is.Len(promrules, 1))
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Expr.StrVal, expr)
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Labels["owner"], "testIngressOwner")
}

func TestIngressAnnotationsRuleBackend(t *testing.T) {
	log.Setup(log.DEBUG_LEVEL)

	client := fake.NewSimpleClientset(testService, testDeployment, testReplicaset, testPod)

	template, err := NewPrometheusRuleTemplateManager("../../kube/config/templates", client)

	expr := `(
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testNamespace",ingress="testRuleBackend",status=~"5.."}[30s]
    )
  )
  /
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testNamespace",ingress="testRuleBackend"}[30s]
    )
  )
) > 0.001
`
	promrules, err := template.CreateFromIngress(testIngressRuleBackend)
	assert.Assert(t, is.Nil(err))
	assert.Assert(t, is.Len(promrules, 1))
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Expr.StrVal, expr)
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Labels["owner"], "testDeploymentOwner")
}

func TestNamesMatch(t *testing.T) {
	services := []string{"test", "test", "test"}
	service := checkNamesMatch(services)
	assert.Equal(t, services[0], service)

	services = []string{"test1", "test2", "test3"}
	service = checkNamesMatch(services)
	assert.Equal(t, "", service)
}
