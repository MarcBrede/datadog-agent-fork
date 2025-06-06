// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostimpl implements a component to generate the 'host' metadata payload (also known as "v5").
package hostimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	hostComp "github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// run the host metadata collector every 1800 seconds (30 minutes)
const defaultCollectInterval = 1800 * time.Second

// the host metadata collector interval can be set through configuration within acceptable bounds
const minAcceptedInterval = 300   // 5min
const maxAcceptedInterval = 14400 // 4h

const providerName = "host"

type host struct {
	log       log.Component
	config    config.Component
	resources resources.Component

	hostname        string
	collectInterval time.Duration
	serializer      serializer.MetricSerializer
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newHostProvider),
	)
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Resources  resources.Component
	Serializer serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp                 hostComp.Component
	MetadataProvider     runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
	GohaiEndpoint        api.AgentEndpointProvider
}

func newHostProvider(deps dependencies) provides {
	collectInterval := defaultCollectInterval
	confProviders, err := configUtils.GetMetadataProviders(deps.Config)
	if err != nil {
		deps.Log.Errorf("Error parsing metadata provider configuration, falling back to default behavior: %s", err)
	} else {
		for _, p := range confProviders {
			if p.Name == providerName {
				if p.Interval < minAcceptedInterval || p.Interval > maxAcceptedInterval {
					deps.Log.Errorf("Ignoring host metadata interval: %v is outside of accepted values (min: %v, max: %v)", p.Interval, minAcceptedInterval, maxAcceptedInterval)
					break
				}

				// user configured interval take precedence over the default one
				collectInterval = p.Interval * time.Second
				break
			}
		}
	}

	hname, _ := hostname.Get(context.Background())
	h := host{
		log:             deps.Log,
		config:          deps.Config,
		resources:       deps.Resources,
		hostname:        hname,
		collectInterval: collectInterval,
		serializer:      deps.Serializer,
	}
	return provides{
		Comp:             &h,
		MetadataProvider: runnerimpl.NewProvider(h.collect),
		FlareProvider:    flaretypes.NewProvider(h.fillFlare),
		StatusHeaderProvider: status.NewHeaderInformationProvider(StatusProvider{
			Config: h.config,
		}),
		Endpoint:      api.NewAgentEndpointProvider(h.writePayloadAsJSON, "/metadata/v5", "GET"),
		GohaiEndpoint: api.NewAgentEndpointProvider(h.writeGohaiPayload, "/metadata/gohai", "GET"),
	}
}

func (h *host) collect(ctx context.Context) time.Duration {
	payload := h.getPayload(ctx)
	if err := h.serializer.SendHostMetadata(payload); err != nil {
		h.log.Errorf("unable to submit host metadata payload, %s", err)
	}
	return h.collectInterval
}

func (h *host) GetPayloadAsJSON(ctx context.Context) ([]byte, error) {
	return json.MarshalIndent(h.getPayload(ctx), "", "    ")
}

func (h *host) fillFlare(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(filepath.Join("metadata", "host.json"), func() ([]byte, error) { return h.GetPayloadAsJSON(context.Background()) })
}

func (h *host) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	jsonPayload, err := h.GetPayloadAsJSON(context.Background())
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to marshal v5 metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to scrub metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func (h *host) writeGohaiPayload(w http.ResponseWriter, _ *http.Request) {
	payload := gohai.GetPayloadWithProcesses(h.hostname, h.config.GetBool("metadata_ip_resolution_from_hostname"), env.IsContainerized())
	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to marshal gohai metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to scrub gohai metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}
