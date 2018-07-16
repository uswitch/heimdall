package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/uswitch/heimdall/pkg/apis/heimdall.uswitch.com/v1alpha1"
)

var heimPrefix = "com.uswitch.heimdall"

// AlertTemplateManager
// - contains a map of all the templates in the given templates folder
type AlertTemplateManager struct {
	templates map[string]*template.Template
}

// templateParameter
// - struct passed to each alert template
type templateParameter struct {
	Identifier string
	Threshold  string
	Namespace  string
	Name       string
	Host       string
	Value      string
	Ingress    *extensionsv1beta1.Ingress
	Service    *corev1.Service
}

// collectAlerts
// - Accepts a map of Alerts and returns Array
func collectAlerts(alertRules map[string]*v1alpha1.Alert) []*v1alpha1.Alert {
	ret := make([]*v1alpha1.Alert, len(alertRules))

	idx := 0
	for _, v := range alertRules {
		ret[idx] = v
		idx = idx + 1
	}

	return ret
}

// NewAlertTemplateManager
// - Creates a new AlertTemplateManager taking a directory as a string
func NewAlertTemplateManager(directory string) (*AlertTemplateManager, error) {
	templates := map[string]*template.Template{}
	templateFiles, err := filepath.Glob(directory + "/*.tmpl")
	if err != nil {
		return nil, err
	}

	for _, t := range templateFiles {
		tmpl, err := template.ParseFiles(t)
		if err != nil {
			return nil, err
		}

		templates[strings.TrimRight(filepath.Base(t), ".tmpl")] = tmpl
	}

	if len(templates) == 0 {
		return nil, fmt.Errorf("no templates defined")
	}

	return &AlertTemplateManager{templates}, nil
}

// CreateFromIngress
// - Creates all the alerts for a given Ingress
func (a *AlertTemplateManager) CreateFromIngress(ingress *extensionsv1beta1.Ingress) ([]*v1alpha1.Alert, error) {
	ingressIdentifier := fmt.Sprintf("%s.%s", ingress.Namespace, ingress.Name)

	params := &templateParameter{
		Ingress:    ingress,
		Identifier: ingressIdentifier,
		Namespace:  ingress.Namespace,
		Name:       ingress.Name,
	}

	alertRules := map[string]*v1alpha1.Alert{}
	annotations := params.Ingress.GetAnnotations()

	for k, v := range annotations {
		if !strings.HasPrefix(k, heimPrefix) {
			continue
		}

		templateName := strings.TrimLeft(k, fmt.Sprintf("%s/", heimPrefix))
		template, ok := a.templates[templateName]
		if !ok {
			log.Warnf("[ingress][%s] no template for \"%s\"", ingressIdentifier, templateName)
			continue
		}

		params.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			log.Warnf("[ingress][%s] error executing template : %s", ingressIdentifier, err)
			continue
		}

		alert := &v1alpha1.Alert{}

		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 1024).Decode(alert); err != nil {
			log.Warnf("[ingress][%s] error parsing YAML: %s", ingressIdentifier, err)
			continue
		}

		alert.SetOwnerReferences([]metav1.OwnerReference{
			*metav1.NewControllerRef(ingress, schema.GroupVersionKind{
				Group:   extensionsv1beta1.SchemeGroupVersion.Group,
				Version: extensionsv1beta1.SchemeGroupVersion.Version,
				Kind:    "Ingress",
			}),
		})

		alertRules[alert.ObjectMeta.Name] = alert
	}

	return collectAlerts(alertRules), nil
}

// CreateFromService
// - Creates all the alerts for a given Service
func (a *AlertTemplateManager) CreateFromService(svc *corev1.Service) ([]*v1alpha1.Alert, error) {
	identifier := fmt.Sprintf("%s.%s", svc.Namespace, svc.Name)

	params := &templateParameter{
		Service:    svc,
		Identifier: identifier,
		Namespace:  svc.Namespace,
		Name:       svc.Name,
	}

	alertRules := map[string]*v1alpha1.Alert{}
	annotations := params.Service.GetAnnotations()

	for k, v := range annotations {
		if !strings.HasPrefix(k, heimPrefix) {
			continue
		}

		templateName := strings.TrimPrefix(k, fmt.Sprintf("%s/", heimPrefix))
		template, ok := a.templates[templateName]
		if !ok {
			log.Warnf("[service][%s] no template for \"%s\"", identifier, templateName)
			continue
		}

		params.Value = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			log.Warnf("[service][%s] error executing template : %s", identifier, err)
			continue
		}

		alert := &v1alpha1.Alert{}

		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 1024).Decode(alert); err != nil {
			log.Warnf("[service][%s] error parsing YAML: %s", identifier, err)
			continue
		}

		alert.SetOwnerReferences([]metav1.OwnerReference{
			*metav1.NewControllerRef(svc, schema.GroupVersionKind{
				Group:   corev1.SchemeGroupVersion.Group,
				Version: corev1.SchemeGroupVersion.Version,
				Kind:    "Service",
			}),
		})

		alertRules[alert.ObjectMeta.Name] = alert
	}

	return collectAlerts(alertRules), nil
}
