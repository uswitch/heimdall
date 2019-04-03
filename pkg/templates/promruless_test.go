package templates

import (
	"testing"

	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngressAnnotations(t *testing.T) {
	template, _ := NewPrometheusRuleTemplateManager("../../example-prometheusrule-templates")
	ingress := &extensionsv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testicuffs",
			Namespace:   "testificate",
			Annotations: map[string]string{"com.uswitch.heimdall/5xx-rate": "0.001"},
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{
				extensionsv1beta1.IngressRule{Host: "test"},
			},
		},
	}

	expr := `(
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testificate",ingress="testicuffs",status=~"5.."}[30s]
    )
  )
  /
  sum(
    rate(
      nginx_ingress_controller_requests{exported_namespace="testificate",ingress="testicuffs"}[30s]
    )
  )
) > 0.001
`
	promrules, err := template.CreateFromIngress(ingress)
	assert.Assert(t, is.Nil(err))
	assert.Assert(t, is.Len(promrules, 1))
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Expr.StrVal, expr)
}
