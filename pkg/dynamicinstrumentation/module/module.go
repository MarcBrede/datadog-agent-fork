// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"net/http"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	di "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
)

// Module is the dynamic instrumentation system probe module
type Module struct {
	godi *di.GoDI
}

// NewModule creates a new dynamic instrumentation system probe module
func NewModule(_ *Config) (*Module, error) {
	godi, err := di.RunDynamicInstrumentation(&di.DIOptions{
		RateLimitPerProbePerSecond: 1.0,
		OfflineOptions: di.OfflineOptions{
			Offline:          coreconfig.SystemProbe().GetBool("dynamic_instrumentation.offline_mode"),
			ProbesFilePath:   coreconfig.SystemProbe().GetString("dynamic_instrumentation.probes_file_path"),
			SnapshotOutput:   coreconfig.SystemProbe().GetString("dynamic_instrumentation.snapshot_output_file_path"),
			DiagnosticOutput: coreconfig.SystemProbe().GetString("dynamic_instrumentation.diagnostics_output_file_path"),
		},
	})
	if err != nil {
		// FIXME: Logging the error instead of returning it is a temporary fix to avoid
		// having the system-probe get caught in a restart loop when either the environment lacks
		// the bpf feature requirements or the system-probe is not run with needeed permissions.
		// The DI module can be mistakenly turned on as it shares the same environment variable
		// as all DI runtimes, leading to problematic behavior.
		//
		// This means that legitimate errors will be logged, but not cause the module to restart
		// as it should.
		log.Errorf("Failed to start dynamic instrumentation: %v", err)
		return &Module{}, nil
	}
	return &Module{godi}, nil
}

// Close disables the dynamic instrumentation system probe module
func (m *Module) Close() {
	if m.godi == nil {
		log.Info("Could not close dynamic instrumentation module, already closed")
		return
	}
	log.Info("Closing dynamic instrumentation module")
	m.godi.Close()
}

// GetStats returns a map of various metrics about the state of the module
func (m *Module) GetStats() map[string]interface{} {
	if m == nil || m.godi == nil {
		log.Info("Could not get stats from dynamic instrumentation module, closed")
		return map[string]interface{}{}
	}
	debug := map[string]interface{}{}
	stats := m.godi.GetStats()
	debug["PIDEventsCreated"] = stats.PIDEventsCreatedCount
	debug["ProbeEventsCreated"] = stats.ProbeEventsCreatedCount
	return debug
}

// Register creates a health check endpoint for the dynamic instrumentation module
func (m *Module) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests,
		func(w http.ResponseWriter, _ *http.Request) {
			stats := []string{}
			utils.WriteAsJSON(w, stats, utils.CompactOutput)
		}))

	log.Info("Registering dynamic instrumentation module")
	return nil
}
