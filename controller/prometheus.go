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

type input struct {
	Identifier string
	Threshold  string
}

type AlertTemplateManager struct {
	templates map[string]*template.Template
}

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

type Rule struct {
	rule         string
	templateName string
	subject      *v1.ObjectMeta
}

// used to create keys in configmaps so must be a filename safe form
func (r *Rule) Key() string {
	return fmt.Sprintf("%s_%s-%s.rules", r.templateName, r.subject.GetNamespace(), r.subject.GetName())
}

func (a *AlertTemplateManager) Create(ingress *extensionsv1beta1.Ingress) ([]*Rule, error) {
	ingressIdentifier := fmt.Sprintf("%s.%s", ingress.Namespace, ingress.Name)

	alerts := []*Rule{}
	i := input{
		Identifier: ingressIdentifier,
	}

	annotations := ingress.GetAnnotations()
	for k, v := range annotations {
		if !strings.HasPrefix(k, "com.uswitch.heimdall") {
			continue
		}

		templateName := strings.TrimLeft(k, "com.uswitch.heimdall/")
		template, ok := a.templates[templateName]
		if !ok {
			return nil, fmt.Errorf("no template for \"%s\"", templateName)
		}

		i.Threshold = v
		var result bytes.Buffer
		if err := template.Execute(&result, i); err != nil {
			return nil, err
		}

		rule := &Rule{
			rule:         result.String(),
			templateName: templateName,
			subject:      &ingress.ObjectMeta,
		}

		alerts = append(alerts, rule)
	}

	return alerts, nil
}
