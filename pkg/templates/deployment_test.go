package templates

import (
	"testing"

	"github.com/uswitch/heimdall/pkg/log"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	testDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testApp",
			Namespace: "testNamespace",
			Labels:    map[string]string{},
			Annotations: map[string]string{
				ownerAnnotation:       "testDeploymentOwner",
				environmentAnnotation: "testing",
				criticalityAnnotation: "low",
				sensitivityAnnotation: "public",
				"com.uswitch.heimdall/replicas-availability-deployment": "1",
			},
			OwnerReferences: []metav1.OwnerReference{},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32),
			Selector: &metav1.LabelSelector{},
		},
	}
)

func TestDeploymentAnnotations(t *testing.T) {
	log.Setup(log.DEBUG_LEVEL)

	client := fake.NewSimpleClientset()

	template, err := NewPrometheusRuleTemplateManager("../../kube/config/templates", client)

	expr := `kube_deployment_status_replicas_available{namespace="testNamespace", deployment="testApp"}
/
kube_deployment_spec_replicas{namespace="testNamespace", deployment="testApp"} <= 1
`
	promrules, err := template.CreateFromDeployment(testDeployment, "testNamespace")
	assert.Assert(t, is.Nil(err))
	assert.Assert(t, is.Len(promrules, 1))
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Expr.StrVal, expr)
	assert.Equal(t, promrules[0].Spec.Groups[0].Rules[0].Labels["owner"], "testDeploymentOwner")
}
