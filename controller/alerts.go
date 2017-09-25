package controller

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

//AlertTemplateManager contains a map of all the templates in the given templates folder
type AlertTemplateManager struct {
	templates map[string]*template.Template
}

//NewAlertTemplateManager creates a new AlertTemplateManager taking a directory as a string
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

//Rule contains the rule, template name and subject
type Rule struct {
	rule         string
	templateName string
	subject      *v1.ObjectMeta
}

//Key used to create keys in configmaps so must be a filename safe form
func (r *Rule) Key() string {
	return fmt.Sprintf("%s_%s-%s.rules", r.templateName, r.subject.GetNamespace(), r.subject.GetName())
}

type templateParameter struct {
	Identifier string
	Threshold  string
	Namespace  string
	Name       string
	Host       string
	Ingress    *extensionsv1beta1.Ingress
}

//Create makes all the alerts for a given ingress
func (a *AlertTemplateManager) Create(ingress *extensionsv1beta1.Ingress) ([]*Rule, error) {
	ingressIdentifier := fmt.Sprintf("%s.%s", ingress.Namespace, ingress.Name)

	params := &templateParameter{
		Ingress:    ingress,
		Identifier: ingressIdentifier,
		Namespace:  ingress.Namespace,
		Name:       ingress.Name,
	}

	alertRules := map[string]*Rule{}
	annotations := params.Ingress.GetAnnotations()

	for k, v := range annotations {
		if !strings.HasPrefix(k, "com.uswitch.heimdall") {
			continue
		}

		templateName := strings.TrimLeft(k, "com.uswitch.heimdall/")
		template, ok := a.templates[templateName]
		if !ok {
			return nil, fmt.Errorf("no template for \"%s\"", templateName)
		}

		params.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, params); err != nil {
			return nil, err
		}

		alertRule := &Rule{
			rule:         result.String(),
			templateName: templateName,
			subject:      &ingress.ObjectMeta,
		}
		alertRules[alertRule.rule] = alertRule
	}

	ret := make([]*Rule, len(alertRules))
	idx := 0
	for _, v := range alertRules {
		ret[idx] = v
		idx = idx + 1
	}
	return ret, nil
}
