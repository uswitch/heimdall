package templates

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kint "k8s.io/apimachinery/pkg/util/intstr"
)

func TestIngressAnnotations(t *testing.T) {
	a, _ := NewAlertTemplateManager("../../example-alert-templates")
	ingress := &extensionsv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testicuffs",
			Namespace:   "testificate",
			Annotations: map[string]string{"com.uswitch.heimdall/response-msec-threshold": "500"},
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{
				extensionsv1beta1.IngressRule{Host: "testing.com"},
			},
		},
	}

	alerts, err := a.CreateFromIngress(ingress)

	if len(alerts) != 1 || err != nil {
		t.Error(err)
	}

	if !strings.Contains(alerts[0].Spec.Expr, "testificate.testicuffs") {
		t.Error("Unexpected Result : ", alerts[0])
	}
}

func TestServiceAnnotations(t *testing.T) {
	a, _ := NewAlertTemplateManager("../../example-alert-templates")
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-service",
			Namespace:   "my-namespace",
			Annotations: map[string]string{"com.uswitch.heimdall/endpoint-availability": "true"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:       "http",
					Port:       80,
					TargetPort: kint.FromString("8080"),
				},
			},
		},
	}

	alerts, err := a.CreateFromService(svc)

	if len(alerts) != 1 || err != nil {
		t.Error(err)
	}

	alertSpec := alerts[0].Spec
	alertSpecExpr := alertSpec.Expr

	// Check Spec `Expr` namespace
	if !strings.Contains(alertSpecExpr, fmt.Sprintf("namespace=\"%s\"", "my-namespace")) {
		t.Error("Unexpected Expression : ", alertSpecExpr)
	}

	// Check Spec `Expr` endpoint
	if !strings.Contains(alertSpecExpr, fmt.Sprintf("endpoint=\"%s\"", "my-service")) {
		t.Error("Unexpected Expression : ", alertSpecExpr)
	}

	alertSpecFor := alertSpec.For
	// Check `For`
	if !strings.Contains(alertSpecFor, "1m") {
		t.Error("Unexpected Expression : ", alertSpecFor)
	}
}
