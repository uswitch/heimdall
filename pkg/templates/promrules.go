package templates

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	log "github.com/uswitch/heimdall/pkg/log"
	"github.com/uswitch/heimdall/pkg/sentryclient"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var heimPrefix = "com.uswitch.heimdall"

const (
	ownerAnnotation       = "service.rvu.co.uk/owner"
	environmentAnnotation = "service.rvu.co.uk/environment"
	criticalityAnnotation = "service.rvu.co.uk/criticality"
	sensitivityAnnotation = "service.rvu.co.uk/sensitivity"
)

// ClientSetI
// - Clientsets should implement this interface for making requests to find ingress owners
type ClientSetI interface {
	AppsV1() appsv1.AppsV1Interface
	CoreV1() corev1.CoreV1Interface
}

// PrometheusRuleTemplateManager
// - Contains a map of all the templates in the given templates folder
type PrometheusRuleTemplateManager struct {
	clientSet ClientSetI

	templates map[string]*template.Template
}

// NewPrometheusRuleTemplateManager
// - Creates a new PrometheusRuleTemplateManager taking a directory as a string
func NewPrometheusRuleTemplateManager(directory string, clientSet ClientSetI) (*PrometheusRuleTemplateManager, error) {
	templates := map[string]*template.Template{}
	templateFiles, err := filepath.Glob(directory + "/*.tmpl")
	if err != nil {
		sentryclient.SentryErr(err)
		return nil, err
	}

	for _, t := range templateFiles {
		tmpl, err := template.ParseFiles(t)
		if err != nil {
			sentryclient.SentryErr(err)
			return nil, err
		}

		templates[strings.TrimSuffix(filepath.Base(t), ".tmpl")] = tmpl
	}

	log.Sugar.Debugf("%+v", templates)
	if len(templates) == 0 {
		return nil, fmt.Errorf("no templates defined")
	}

	return &PrometheusRuleTemplateManager{clientSet: clientSet, templates: templates}, nil
}

// collectPrometheusRules
// - Accepts a map of PrometheusRules and returns Array
func collectPrometheusRules(prometheusRules map[string]*monitoringv1.PrometheusRule) []*monitoringv1.PrometheusRule {
	ret := make([]*monitoringv1.PrometheusRule, len(prometheusRules))

	idx := 0
	for _, v := range prometheusRules {
		ret[idx] = v
		idx = idx + 1
	}

	return ret
}
