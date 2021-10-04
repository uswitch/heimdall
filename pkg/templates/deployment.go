package templates

import (
	"bytes"
	"fmt"
	"strings"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/uswitch/heimdall/pkg/log"
	"github.com/uswitch/heimdall/pkg/sentryclient"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// templateParameterDeployment
// - struct passed to each promrule template
type templateParameterDeployment struct {
	Identifier          string
	Threshold           string
	Namespace           string
	NamespacePrometheus string
	Name                string
	Host                string
	Value               string
	GeneratedLabels     string
	NSPrometheus        string
	Owner               string
	Environment         string
	Criticality         string
	Sensitivity         string
	Deployment          *apps.Deployment
}

// CreateFromDeployment
// - Creates all the promRules for a given Deployment
func (a *PrometheusRuleTemplateManager) CreateFromDeployment(deployment *apps.Deployment, depNamespacePrometheus string) ([]*monitoringv1.PrometheusRule, error) {
	logger := log.Sugar.With("name", deployment.Name, "namespace", deployment.Namespace, "kind", deployment.Kind)
	deploymentIdentifier := fmt.Sprintf("%s.%s", deployment.Namespace, deployment.Name)

	owner := deployment.GetAnnotations()[ownerAnnotation]
	criticality := deployment.GetAnnotations()[criticalityAnnotation]
	environment := deployment.GetAnnotations()[environmentAnnotation]
	sensitivity := deployment.GetAnnotations()[sensitivityAnnotation]
	selectorMap := deployment.Spec.Selector.MatchLabels

	var builder strings.Builder
	for key, value := range selectorMap {
		fmt.Fprintf(&builder, ",%s=\"%s\"", key, value)
	}
	generatedLabels := builder.String()

	logger.Debugw("generated labels", "labels", generatedLabels)

	params := &templateParameterDeployment{
		Deployment:      deployment,
		Identifier:      deploymentIdentifier,
		Namespace:       deployment.Namespace,
		Name:            deployment.Name,
		GeneratedLabels: generatedLabels,
		Owner:           owner,
		Criticality:     criticality,
		Environment:     environment,
		Sensitivity:     sensitivity,
		NSPrometheus:    depNamespacePrometheus,
	}

	prometheusRules := map[string]*monitoringv1.PrometheusRule{}
	annotations := params.Deployment.GetAnnotations()

	for k, v := range annotations {
		if !strings.HasPrefix(k, heimPrefix) {
			continue
		}

		templateName := strings.TrimLeft(k, fmt.Sprintf("%s/", heimPrefix))
		logger.Infow("template selected", "template", templateName)
		template, ok := a.templates[templateName]
		if !ok {
			warnMessage := fmt.Sprintf("[deployment][%s] no template for \"%s\"", deploymentIdentifier, templateName)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		params.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			warnMessage := fmt.Sprintf("[deployment][%s] error executing template : %s", deploymentIdentifier, err)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		promrule := &monitoringv1.PrometheusRule{}

		if err := yaml.NewYAMLOrJSONDecoder(&result, 1024).Decode(promrule); err != nil {
			warnMessage := fmt.Sprintf("[deployment][%s] error parsing YAML: %s", deploymentIdentifier, err)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		promrule.SetOwnerReferences([]metav1.OwnerReference{
			*metav1.NewControllerRef(deployment, schema.GroupVersionKind{
				Group:   apps.SchemeGroupVersion.Group,
				Version: apps.SchemeGroupVersion.Version,
				Kind:    "Deployment",
			}),
		})

		prometheusRules[promrule.ObjectMeta.Name] = promrule
	}

	return collectPrometheusRules(prometheusRules), nil
}
