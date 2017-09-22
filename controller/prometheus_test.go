package controller

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
)

func TestCreateAlert(t *testing.T) {
	ingress := &extensionsv1beta1.Ingress{}
	a, err := NewAlertTemplateManager("templates")
	if err != nil {
		t.Fatal(err)
	}

	if alerts, err := a.Create(ingress); len(alerts) > 0 || err != nil {
		t.Fatal("Nil points")
	}
}

func TestIngressAnnotations(t *testing.T) {
	a, _ := NewAlertTemplateManager("templates")
	ingress := &extensionsv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testicuffs",
			Namespace:   "testificate",
			Annotations: map[string]string{"com.uswitch.heimdall/response-msec-threshold": "500"},
		},
	}

	if alerts, err := a.Create(ingress); len(alerts) != 1 || err != nil {
		t.Error(err)
	} else if !strings.Contains(alerts[0].rule, "testificate.testicuffs") {
		t.Error("Unexpected Result : ", alerts[0])
	}

}
