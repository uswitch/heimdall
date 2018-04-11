package prometheus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/uswitch/heimdall/pkg/apis/heimdall.uswitch.com/v1alpha1"
)

type rule struct {
	Alert       string `json:"alert"`
	Expr        string `json:"expr"`
	For         string `json:"for"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type group struct {
	Name  string `json:"name"`
	Rules []*rule `json:"rules"`
}

type container struct {
	Groups []*group `json:"groups"`
}

func ToYAML(alert *v1alpha1.Alert) (string, error) {

	filteredAnnotations := map[string]string{}

	for k, v := range alert.GetAnnotations() {
		if strings.HasPrefix(k, "com.uswitch.heimdall/") {
			filteredAnnotations[strings.TrimPrefix(k, "com.uswitch.heimdall/")] = v
		}
	}

	labels := map[string]string{}

	for k, v := range alert.GetLabels() {
		labels[k] = v
	}

	labels["namespace"] = alert.GetNamespace()
	labels["name"] = alert.GetName()

	c := container{
		Groups: []*group{
			&group{
				Name: fmt.Sprintf("%s.rules", alert.GetName()),
				Rules: []*rule{
					&rule{
						Alert: alert.GetName(),
						Expr: alert.Spec.Expr,
						For: alert.Spec.For,
						Labels: alert.GetLabels(),
					},
				},
			},
		},
	}

	out, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(out), nil
}
