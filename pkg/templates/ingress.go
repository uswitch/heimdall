package templates

import (
	"bytes"
	"fmt"
	"strings"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/uswitch/heimdall/pkg/log"
	"github.com/uswitch/heimdall/pkg/sentryclient"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	logger := log.Sugar.With("name", ingress.Name, "namespace", ingress.Namespace, "kind", ingress.Kind)
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

		templateName := strings.TrimLeft(k, fmt.Sprintf("%s/", heimPrefix))
		template, ok := a.templates[templateName]
		if !ok {
			warnMessage := fmt.Sprintf("[ingress][%s] no template for \"%s\"", ingressIdentifier, templateName)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		params, err := a.resolveIngressOwner(params)
		if err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error finding owner: %s", ingressIdentifier, err)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
		}

		params.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error executing template: %s", ingressIdentifier, err)
			logger.Warnf(warnMessage)
			sentryclient.SentryMessage(warnMessage)
			continue
		}

		promrule := &monitoringv1.PrometheusRule{}

		if err := yaml.NewYAMLOrJSONDecoder(&result, 1024).Decode(promrule); err != nil {
			warnMessage := fmt.Sprintf("[ingress][%s] error parsing YAML: %s", ingressIdentifier, err)
			logger.Warnf(warnMessage)
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

func (a *PrometheusRuleTemplateManager) findServiceDeployment(serviceName, namespace string) (*metav1.ObjectMeta, error) {
	service, err := a.clientSet.CoreV1().Services(namespace).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting service: %v", err)
	}

	podOwners, err := a.listPodOwnerReferences(service.Spec.Selector, namespace)
	if err != nil {
		return nil, fmt.Errorf("error getting pod owner references: %v", err)
	}

	replicasetOwners, err := a.getReplicasetOwnerReferences(podOwners, namespace)
	if err != nil {
		return nil, fmt.Errorf("error getting replicaset owner references: %v", err)
	}

	deployments, err := a.getDeployments(replicasetOwners, namespace)
	if err != nil {
		return nil, fmt.Errorf("error getting deployments: %v", err)
	}

	if len(deployments) != 1 {
		return nil, fmt.Errorf("could not find 1 deployment for service, got: %v", len(deployments))
	}

	return deployments[0], nil
}

func (a *PrometheusRuleTemplateManager) listPodOwnerReferences(selector map[string]string, namespace string) (map[string]metav1.OwnerReference, error) {
	var podsMeta []*metav1.ObjectMeta

	set := labels.Set(selector)
	listOptions := metav1.ListOptions{LabelSelector: set.AsSelector().String()}

	pods, err := a.clientSet.CoreV1().Pods(namespace).List(listOptions)
	if err != nil {
		return nil, fmt.Errorf("error getting pods: %v", err)
	}

	for _, pod := range pods.Items {
		podsMeta = append(podsMeta, &pod.ObjectMeta)
	}

	return uniqueOwnerReferences(podsMeta), nil
}

func (a *PrometheusRuleTemplateManager) getReplicasetOwnerReferences(podOwners map[string]metav1.OwnerReference, namespace string) (map[string]metav1.OwnerReference, error) {
	var replicasetsMeta []*metav1.ObjectMeta

	for _, owner := range podOwners {
		replicasetMeta, err := a.getAppsObjectMeta(owner.Name, namespace, owner.Kind)
		if err != nil {
			return nil, fmt.Errorf("error getting object meta for replicaset: %v", err)
		}

		replicasetsMeta = append(replicasetsMeta, replicasetMeta)
	}

	return uniqueOwnerReferences(replicasetsMeta), nil
}

func (a *PrometheusRuleTemplateManager) getDeployments(replicasetOwners map[string]metav1.OwnerReference, namespace string) ([]*metav1.ObjectMeta, error) {
	uniqDeployments := make(map[string]*metav1.ObjectMeta)

	for _, owner := range replicasetOwners {
		deploymentMeta, err := a.getAppsObjectMeta(owner.Name, namespace, owner.Kind)
		if err != nil {
			return nil, fmt.Errorf("error getting object meta for deployment: %v", err)
		}

		uniqDeployments[fmt.Sprintf("%s/%s/%s", deploymentMeta.Name, deploymentMeta.Namespace, deploymentMeta.UID)] = deploymentMeta
	}

	var deployments []*metav1.ObjectMeta
	for _, deployment := range uniqDeployments {
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (a *PrometheusRuleTemplateManager) getAppsObjectMeta(name, namespace, kind string) (*metav1.ObjectMeta, error) {
	switch {
	default:
		return nil, fmt.Errorf("got unrecognised apps kind: %v", kind)
	case kind == "Deployment":
		deployment, err := a.clientSet.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting deployment: %v", err)
		}
		return &deployment.ObjectMeta, nil

	case kind == "ReplicaSet":
		replicaset, err := a.clientSet.AppsV1().ReplicaSets(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting replicaset: %v", err)
		}
		return &replicaset.ObjectMeta, nil
	}
}

func uniqueOwnerReferences(objects []*metav1.ObjectMeta) map[string]metav1.OwnerReference {
	uniqueOwnerReferences := make(map[string]metav1.OwnerReference)

	for _, object := range objects {
		for _, owner := range object.OwnerReferences {
			uniqueOwnerReferences[fmt.Sprintf("%s/%s/%s", owner.APIVersion, owner.Kind, owner.Name)] = owner
		}
	}
	return uniqueOwnerReferences
}
