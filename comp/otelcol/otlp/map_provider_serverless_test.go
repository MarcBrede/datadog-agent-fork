// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && serverless && test
// +build otlp,serverless,test

package otlp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
)

func TestNewMap(t *testing.T) {
	tests := []struct {
		name string
		pcfg PipelineConfig
		ocfg map[string]interface{}
	}{
		{
			name: "only gRPC, only Traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"verbosity": "none",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"verbosity": "none",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
						"metrics": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, invalid verbosity (ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"verbosity": "foo",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
						"metrics": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "with both",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 5678),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"verbosity": "none",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
							"http": map[string]interface{}{
								"endpoint": "bindhost:5678",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                              1500,
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": false,
					"histograms": map[string]interface{}{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"verbosity": "none",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                              1500,
							"resource_attributes_as_tags":            false,
							"instrumentation_scope_metadata_as_tags": false,
							"histograms": map[string]interface{}{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"metrics": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, only Traces, logging with normal verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"verbosity": "normal",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
					"debug": map[string]interface{}{
						"verbosity": "normal",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics, logging with detailed verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                   1500,
					"resource_attributes_as_tags": false,
					"histograms": map[string]interface{}{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"verbosity": "detailed",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                   1500,
							"resource_attributes_as_tags": false,
							"histograms": map[string]interface{}{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
					},
					"debug": map[string]interface{}{
						"verbosity": "detailed",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"metrics": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"serializer", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, logging with basic verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                   2000,
					"resource_attributes_as_tags": true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"verbosity": "basic",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]interface{}{
							"enabled": false,
						},
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                   2000,
							"resource_attributes_as_tags": true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
					"debug": map[string]interface{}{
						"verbosity": "basic",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp", "debug"},
						},
						"metrics": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"serializer", "debug"},
						},
					},
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg, err := buildMap(testInstance.pcfg)
			require.NoError(t, err)
			tcfg := confmap.NewFromStringMap(testInstance.ocfg)
			assert.Equal(t, tcfg.ToStringMap(), cfg.ToStringMap())
		})
	}
}
