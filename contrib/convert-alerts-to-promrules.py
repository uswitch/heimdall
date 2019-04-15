#!/usr/bin/env python

import yaml
import sys

alerts = yaml.safe_load_all(sys.stdin.read())
promrules = []

for alert in alerts:
    promrule = {
        'apiVersion': 'monitoring.coreos.com/v1',
        'kind': 'PrometheusRule',
        'metadata': {
            'name': alert['metadata']['name'],
            'namespace': alert['metadata']['namespace'],
            'labels': {
                'role': 'alert-rules',
            },
        },
        'spec': {
            'groups': [{
                'name': alert['metadata']['name'] + '.rules',
                'rules': [{
                    'alert': alert['metadata']['name'],
                    'annotations': {
                        'summary': alert['metadata']['annotations']['heimdall.uswitch.com/summary']
                    },
                    'expr': alert['spec']['expr'],
                    'for': alert['spec']['for'],
                    'labels': {
                        'name': alert['metadata']['name'],
                        'namespace': alert['metadata']['namespace'],
                    }
                }]
            }]
        }
    }

    for label_name, label_value in alert.get('metadata', []).get('labels', {}).items():
        promrule['spec']['groups'][0]['rules'][0]['labels'][label_name] = label_value

    promrules.append(promrule)

print(yaml.dump_all(promrules, explicit_start=True, default_flow_style=False))
