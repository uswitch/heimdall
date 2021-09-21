package templates

import (
	"bytes"
	"fmt"
	"strings"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/uswitch/heimdall/pkg/log"
	"github.com/uswitch/heimdall/pkg/sentryclient"
	apps "k8s.io/api/apps/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type templateParameterIngress struct {
	Ingress        *extensionsv1beta1.Ingress
	Identifier     string
	Threshold      string
	Namespace      string
	Name           string
	Host           string
	Value          string
	Owner          string
	Environment    string
	Criticality    string
	Sensitivity    string
	BackendService string
}

// CreateFromIngress
// - Creates all the promRules for a given Ingress
func (a *PrometheusRuleTemplateManager) CreateFromIngress(ingress *extensionsv1beta1.Ingress) ([]*monitoringv1.PrometheusRule, error) {
	ingressIdentifier := fmt.Sprintf("%s.%s", ingress.Namespace, ingress.Name)

	params := &templateParameterIngress{
		Ingress:     ingress,
		Identifier:  ingressIdentifier,
		Namespace:   ingress.Namespace,
		Name:        ingress.Name,
		Owner:       ingress.GetAnnotations()[ownerAnnotation],
		Environment: ingress.GetAnnotations()[environmentAnnotation],
		Criticality: ingress.GetAnnotations()[criticalityAnnotation],
		Sensitivity: ingress.GetAnnotations()[sensitivityAnnotation],
	}

	prometheusRules := map[string]*monitoringv1.PrometheusRule{}
	annotations := ingress.GetAnnotations()

	for k, v := range annotations {
		if !strings.HasPrefix(k, heimPrefix) {
			continue
		}

		params, err := a.resolveIngressOwner(params)
		if err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error finding owner: %s", ingressIdentifier, err)
			log.Sugar.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
		}

		templateName := strings.TrimLeft(k, fmt.Sprintf("%s/", heimPrefix))
		template, ok := a.templates[templateName]
		if !ok {
			warnMessage := fmt.Sprintf("[ingress][%s] no template for \"%s\"", ingressIdentifier, templateName)
			log.Sugar.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		params.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error executing template: %s", ingressIdentifier, err)
			log.Sugar.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		promrule := &monitoringv1.PrometheusRule{}

		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 1024).Decode(promrule); err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error parsing YAML: %s", ingressIdentifier, err)
			log.Sugar.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		promrule.SetOwnerReferences([]metav1.OwnerReference{
			*metav1.NewControllerRef(ingress, schema.GroupVersionKind{
				Group:   extensionsv1beta1.SchemeGroupVersion.Group,
				Version: extensionsv1beta1.SchemeGroupVersion.Version,
				Kind:    "Ingress",
			}),
		})

		prometheusRules[promrule.ObjectMeta.Name] = promrule
	}

	return collectPrometheusRules(prometheusRules), nil
}

func (a *PrometheusRuleTemplateManager) resolveIngressOwner(params *templateParameterIngress) (*templateParameterIngress, error) {
	if len(params.Owner) != 0 {
		return params, nil
	}

	if err := params.getIngressService(); err != nil {
		return params, fmt.Errorf("error getting service for ingress: %v", err)
	}

	deployment, err := a.findServiceDeployment(params.BackendService, params.Namespace)
	if err != nil {
		return params, fmt.Errorf("error getting deployment for service: %v", err)
	}

	params.Owner = deployment.GetAnnotations()[ownerAnnotation]
	params.Environment = deployment.GetAnnotations()[environmentAnnotation]
	params.Criticality = deployment.GetAnnotations()[criticalityAnnotation]
	params.Sensitivity = deployment.GetAnnotations()[sensitivityAnnotation]

	return params, nil
}

func (a *PrometheusRuleTemplateManager) findServiceDeployment(serviceName, namespace string) (*apps.Deployment, error) {
	options := metav1.GetOptions{}
	service, err := a.clientSet.CoreV1().Services(namespace).Get(serviceName, options)
	if err != nil {
		return nil, fmt.Errorf("error getting service: %v", err)
	}

	deployment, err := a.clientSet.AppsV1().Deployments(namespace).Get(service.Spec.Selector["app"], options)
	if err != nil {
		return nil, fmt.Errorf("error getting deployment: %v", err)
	}

	return deployment, nil
}

func (p *templateParameterIngress) getIngressService() error {
	switch {
	case len(p.Ingress.Spec.Rules) == 0:
		if p.Ingress.Spec.Backend != nil {
			p.BackendService = p.Ingress.Spec.Backend.ServiceName
		}

	case len(p.Ingress.Spec.Rules) != 0:
		var services []string
		for _, r := range p.Ingress.Spec.Rules {
			for _, s := range r.IngressRuleValue.HTTP.Paths {
				services = append(services, s.Backend.ServiceName)
			}
		}

		if p.Ingress.Spec.Backend != nil {
			services = append(services, p.Ingress.Spec.Backend.ServiceName)
		}

		p.BackendService = checkNamesMatch(services)
	}

	if p.BackendService == "" {
		return fmt.Errorf("could not find single service for ingress")
	}

	return nil
}

func checkNamesMatch(services []string) string {
	for i := 1; i < len(services); i++ {
		if services[i] != services[0] {
			return ""
		}
	}
	return services[0]
}
