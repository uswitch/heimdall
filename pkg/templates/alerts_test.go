package templates

import (
	"strings"
	"testing"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if alerts, err := a.Create(ingress); len(alerts) != 1 || err != nil {
		t.Error(err)
	} else if !strings.Contains(alerts[0].Spec.Expr, "testificate.testicuffs") {
		t.Error("Unexpected Result : ", alerts[0])
	}
}
