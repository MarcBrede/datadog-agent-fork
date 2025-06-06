// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package docker implements the Docker Workloadmeta collector.
package docker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	errorspkg "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	collectorID   = "docker"
	componentName = "workloadmeta-docker"
)

// imageEventActionSbom is an event that we set to create a fake docker event.
const imageEventActionSbom = events.Action("sbom")

type resolveHook func(ctx context.Context, co container.InspectResponse) (string, error)

type collector struct {
	id      string
	store   workloadmeta.Component
	catalog workloadmeta.AgentType

	dockerUtil        *docker.DockerUtil
	containerEventsCh <-chan *docker.ContainerEvent
	imageEventsCh     <-chan *docker.ImageEvent

	// Images are updated from 2 goroutines: the one that handles docker
	// events, and the one that extracts SBOMS.
	// This mutex is used to handle images one at a time to avoid
	// inconsistencies like trying to set an SBOM for an image that is being
	// deleted.
	handleImagesMut sync.Mutex

	// SBOM Scanning
	sbomScanner *scanner.Scanner //nolint: unused
}

// NewCollector returns a new docker collector provider and an error
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			catalog: workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Docker) {
		return errorspkg.NewDisabled(componentName, "Agent is not running on Docker")
	}

	c.store = store

	var err error
	c.dockerUtil, err = docker.GetDockerUtil()
	if err != nil {
		return err
	}

	if err = c.startSBOMCollection(ctx); err != nil {
		return err
	}

	filter, err := containers.GetPauseContainerFilter()
	if err != nil {
		log.Warnf("Can't get pause container filter, no filtering will be applied: %v", err)
	}

	c.containerEventsCh, c.imageEventsCh, err = c.dockerUtil.SubscribeToEvents(componentName, filter)
	if err != nil {
		return err
	}

	err = c.generateEventsFromImageList(ctx)
	if err != nil {
		return err
	}

	err = c.generateEventsFromContainerList(ctx, filter)
	if err != nil {
		return err
	}

	go c.stream(ctx)

	return nil
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func (c *collector) stream(ctx context.Context) {
	health := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)

	for {
		select {
		case <-health.C:

		case ev := <-c.containerEventsCh:
			err := c.handleContainerEvent(ctx, ev)
			if err != nil {
				log.Warnf("%s", err.Error())
			}

		case ev := <-c.imageEventsCh:
			err := c.handleImageEvent(ctx, ev, nil)
			if err != nil {
				log.Warnf("%s", err.Error())
			}

		case <-ctx.Done():
			var err error

			err = c.dockerUtil.UnsubscribeFromContainerEvents("DockerCollector")
			if err != nil {
				log.Warnf("error unsubscribing from container events: %s", err)
			}

			err = health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}

			cancel()

			return
		}
	}
}

func (c *collector) generateEventsFromContainerList(ctx context.Context, filter *containers.Filter) error {
	if c.store == nil {
		return errors.New("Start was not called")
	}

	containers, err := c.dockerUtil.RawContainerListWithFilter(ctx, container.ListOptions{}, filter, c.store)
	if err != nil {
		return err
	}

	evs := make([]workloadmeta.CollectorEvent, 0, len(containers))
	for _, container := range containers {
		ev, err := c.buildCollectorEvent(ctx, &docker.ContainerEvent{
			ContainerID: container.ID,
			Action:      events.ActionStart,
		})
		if err != nil {
			log.Warnf("%s", err.Error())
			continue
		}

		evs = append(evs, ev)
	}

	if len(evs) > 0 {
		c.store.Notify(evs)
	}

	return nil
}

func (c *collector) generateEventsFromImageList(ctx context.Context) error {
	images, err := c.dockerUtil.Images(ctx, true)
	if err != nil {
		return err
	}

	events := make([]workloadmeta.CollectorEvent, 0, len(images))

	for _, img := range images {
		imgMetadata, err := c.getImageMetadata(ctx, img.ID, nil)
		if err != nil {
			log.Warnf("%s", err.Error())
			continue
		}

		event := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: imgMetadata,
		}

		events = append(events, event)
	}

	if len(events) > 0 {
		c.store.Notify(events)
	}

	return nil
}

func (c *collector) handleContainerEvent(ctx context.Context, ev *docker.ContainerEvent) error {
	event, err := c.buildCollectorEvent(ctx, ev)
	if err != nil {
		return err
	}

	c.store.Notify([]workloadmeta.CollectorEvent{event})

	return nil
}

func (c *collector) buildCollectorEvent(ctx context.Context, ev *docker.ContainerEvent) (workloadmeta.CollectorEvent, error) {
	event := workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceRuntime,
	}

	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   ev.ContainerID,
	}

	switch ev.Action {
	case events.ActionStart, events.ActionRename, events.ActionHealthStatusRunning, events.ActionHealthStatusHealthy, events.ActionHealthStatusUnhealthy, events.ActionHealthStatus:
		container, err := c.dockerUtil.InspectNoCache(ctx, ev.ContainerID, false)
		if err != nil {
			return event, fmt.Errorf("could not inspect container %q: %s", ev.ContainerID, err)
		}

		if ev.Action != events.ActionStart && !container.State.Running {
			return event, fmt.Errorf("received event: %s on dead container: %q, discarding", ev.Action, ev.ContainerID)
		}

		var createdAt time.Time
		if container.Created != "" {
			createdAt, err = time.Parse(time.RFC3339, container.Created)
			if err != nil {
				log.Debugf("Could not parse creation time '%q' for container %q: %s", container.Created, container.ID, err)
			}
		}

		var startedAt time.Time
		if container.State.StartedAt != "" {
			startedAt, err = time.Parse(time.RFC3339, container.State.StartedAt)
			if err != nil {
				log.Debugf("Cannot parse StartedAt %q for container %q: %s", container.State.StartedAt, container.ID, err)
			}
		}

		var finishedAt time.Time
		if container.State.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339, container.State.FinishedAt)
			if err != nil {
				log.Debugf("Cannot parse FinishedAt %q for container %q: %s", container.State.FinishedAt, container.ID, err)
			}
		}

		event.Type = workloadmeta.EventTypeSet
		event.Entity = &workloadmeta.Container{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:   strings.TrimPrefix(container.Name, "/"),
				Labels: container.Config.Labels,
			},
			Image:   extractImage(ctx, container, c.dockerUtil.ResolveImageNameFromContainer, c.store),
			EnvVars: extractEnvVars(container.Config.Env),
			Ports:   extractPorts(container),
			Runtime: workloadmeta.ContainerRuntimeDocker,
			State: workloadmeta.ContainerState{
				Running:    container.State.Running,
				Status:     extractStatus(container.State),
				Health:     extractHealth(container.Config.Labels, container.State.Health),
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
				CreatedAt:  createdAt,
			},
			NetworkIPs:   extractNetworkIPs(container.NetworkSettings.Networks),
			Hostname:     container.Config.Hostname,
			PID:          container.State.Pid,
			RestartCount: container.RestartCount,
		}

	case events.ActionDie, docker.ActionDied:
		var exitCode *int64
		if exitCodeString, found := ev.Attributes["exitCode"]; found {
			exitCodeInt, err := strconv.ParseInt(exitCodeString, 10, 64)
			if err != nil {
				log.Debugf("Cannot convert exit code %q: %v", exitCodeString, err)
			} else {
				exitCode = pointer.Ptr(exitCodeInt)
			}
		}

		event.Type = workloadmeta.EventTypeUnset
		event.Entity = &workloadmeta.Container{
			EntityID: entityID,
			State: workloadmeta.ContainerState{
				Running:    false,
				FinishedAt: ev.Timestamp,
				ExitCode:   exitCode,
			},
		}

	default:
		return event, fmt.Errorf("unknown action type %q, ignoring", ev.Action)
	}

	return event, nil
}

func extractImage(ctx context.Context, container container.InspectResponse, resolve resolveHook, store workloadmeta.Component) workloadmeta.ContainerImage {
	imageSpec := container.Config.Image
	image := workloadmeta.ContainerImage{
		RawName: imageSpec,
		Name:    imageSpec,
	}

	var (
		name      string
		registry  string
		shortName string
		tag       string
		err       error
	)

	if strings.Contains(imageSpec, "@sha256") {
		name, registry, shortName, tag, err = pkgcontainersimage.SplitImageName(imageSpec)
		if err != nil {
			log.Debugf("cannot split image name %q for container %q: %s", imageSpec, container.ID, err)
		}
	}

	if name == "" && tag == "" {
		resolvedImageSpec, err := resolve(ctx, container)
		if err != nil {
			log.Debugf("cannot resolve image name %q for container %q: %s", imageSpec, container.ID, err)
			return image
		}

		name, registry, shortName, tag, err = pkgcontainersimage.SplitImageName(resolvedImageSpec)
		if err != nil {
			log.Debugf("cannot split image name %q for container %q: %s", resolvedImageSpec, container.ID, err)

			// fallback and try to parse the original imageSpec anyway
			if errors.Is(err, pkgcontainersimage.ErrImageIsSha256) {
				name, registry, shortName, tag, err = pkgcontainersimage.SplitImageName(imageSpec)
				if err != nil {
					log.Debugf("cannot split image name %q for container %q: %s", imageSpec, container.ID, err)
					return image
				}
			} else {
				return image
			}
		}
	}

	image.Name = name
	image.Registry = registry
	image.ShortName = shortName
	image.Tag = tag
	image.ID = container.Image
	image.RepoDigest = util.ExtractRepoDigestFromImage(image.ID, image.Registry, store) // "sha256:digest"
	return image
}

func extractEnvVars(env []string) map[string]string {
	envMap := make(map[string]string)

	for _, e := range env {
		envSplit := strings.SplitN(e, "=", 2)
		if len(envSplit) != 2 {
			log.Debugf("cannot parse env var from string: %q", e)
			continue
		}

		if containers.EnvVarFilterFromConfig().IsIncluded(envSplit[0]) {
			envMap[envSplit[0]] = envSplit[1]
		}
	}

	return envMap
}

func extractPorts(container container.InspectResponse) []workloadmeta.ContainerPort {
	var ports []workloadmeta.ContainerPort

	// yes, the code in both branches is exactly the same. unfortunately.
	// Ports and ExposedPorts are different types.
	switch {
	case len(container.NetworkSettings.Ports) > 0:
		for p := range container.NetworkSettings.Ports {
			ports = append(ports, extractPort(p)...)
		}
	case len(container.Config.ExposedPorts) > 0:
		for p := range container.Config.ExposedPorts {
			ports = append(ports, extractPort(p)...)
		}
	}

	return ports
}

func extractPort(port nat.Port) []workloadmeta.ContainerPort {
	var output []workloadmeta.ContainerPort

	// Try to parse a port range, eg. 22-25
	first, last, err := port.Range()
	if err != nil {
		log.Debugf("cannot get port range from nat.Port: %s", err)
		return output
	}

	if last > first {
		output = make([]workloadmeta.ContainerPort, 0, last-first+1)
		for p := first; p <= last; p++ {
			output = append(output, workloadmeta.ContainerPort{
				Port:     p,
				Protocol: port.Proto(),
			})
		}

		return output
	}

	// Try to parse a single port (most common case)
	p := port.Int()
	if p > 0 {
		output = []workloadmeta.ContainerPort{
			{
				Port:     p,
				Protocol: port.Proto(),
			},
		}
	}

	return output
}

func extractNetworkIPs(networks map[string]*network.EndpointSettings) map[string]string {
	networkIPs := make(map[string]string)

	for net, settings := range networks {
		if len(settings.IPAddress) > 0 {
			networkIPs[net] = settings.IPAddress
		}
	}

	return networkIPs
}

func extractStatus(containerState *container.State) workloadmeta.ContainerStatus {
	if containerState == nil {
		return workloadmeta.ContainerStatusUnknown
	}

	switch containerState.Status {
	case "created":
		return workloadmeta.ContainerStatusCreated
	case "running":
		return workloadmeta.ContainerStatusRunning
	case "paused":
		return workloadmeta.ContainerStatusPaused
	case "restarting":
		return workloadmeta.ContainerStatusRestarting
	case "removing", "exited", "dead":
		return workloadmeta.ContainerStatusStopped
	}

	return workloadmeta.ContainerStatusUnknown
}

func extractHealth(containerLabels map[string]string, containerHealth *container.Health) workloadmeta.ContainerHealth {
	// When we're running in Kubernetes, do not report health from Docker but from Kubelet readiness
	if _, ok := containerLabels[kubernetes.CriContainerNamespaceLabel]; ok {
		return ""
	}

	if containerHealth == nil {
		return workloadmeta.ContainerHealthUnknown
	}

	switch containerHealth.Status {
	case container.NoHealthcheck, container.Starting:
		return workloadmeta.ContainerHealthUnknown
	case container.Healthy:
		return workloadmeta.ContainerHealthHealthy
	case container.Unhealthy:
		return workloadmeta.ContainerHealthUnhealthy
	}

	return workloadmeta.ContainerHealthUnknown
}

func (c *collector) handleImageEvent(ctx context.Context, event *docker.ImageEvent, bom *workloadmeta.SBOM) error {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()

	switch event.Action {
	case events.ActionPull, events.ActionTag, events.ActionUnTag, imageEventActionSbom:
		imgMetadata, err := c.getImageMetadata(ctx, event.ImageID, bom)
		if err != nil {
			return fmt.Errorf("could not get image metadata for image %q: %w", event.ImageID, err)
		}

		workloadmetaEvent := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: imgMetadata,
		}

		c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})
	case events.ActionDelete:
		workloadmetaEvent := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   event.ImageID,
				},
			},
		}

		c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})
	}

	return nil
}

func (c *collector) getImageMetadata(ctx context.Context, imageID string, newSBOM *workloadmeta.SBOM) (*workloadmeta.ContainerImageMetadata, error) {
	imgInspect, err := c.dockerUtil.ImageInspect(ctx, imageID)
	if err != nil {
		return nil, err
	}

	imageHistory, err := c.dockerUtil.ImageHistory(ctx, imageID)
	if err != nil {
		// Not sure if it's possible to get the image history in all the
		// environments. If it's not, return the rest of metadata instead of an
		// error.
		log.Warnf("error getting image history: %s", err)
	}

	labels := make(map[string]string)
	if imgInspect.Config != nil {
		labels = imgInspect.Config.Labels
	}

	imageName := c.dockerUtil.GetPreferredImageName(
		imgInspect.ID,
		imgInspect.RepoTags,
		imgInspect.RepoDigests,
	)

	sbom := newSBOM
	// We can get "create" events for images that already exist. That happens
	// when the same image is referenced with different names. For example,
	// datadog/agent:latest and datadog/agent:7 might refer to the same image.
	// Also, in some environments (at least with Kind), pulling an image like
	// datadog/agent:latest creates several events: in one of them the image
	// name is a digest, in other is something with the same format as
	// datadog/agent:7, and sometimes there's a temporary name prefixed with
	// "import-".
	// When that happens, give precedence to the name with repo and tag instead
	// of the name that includes a digest. This is just to show names that are
	// more user-friendly (the digests are already present in other attributes
	// like ID, and repo digest).
	existingImg, err := c.store.GetImage(imageID)
	if err == nil {
		if strings.Contains(imageName, "sha256:") && !strings.Contains(existingImg.Name, "sha256:") {
			imageName = existingImg.Name
		}

		if sbom == nil && existingImg.SBOM.Status != workloadmeta.Pending {
			sbom = existingImg.SBOM
		}
	}

	if sbom == nil {
		sbom = &workloadmeta.SBOM{
			Status: workloadmeta.Pending,
		}
	}

	// The CycloneDX should contain the RepoTags and RepoDigests but the scanner might
	// not be able to inject them. For example, if we use the scanner from filesystem or
	// if the `imgMeta` object does not contain all the metadata when it is sent.
	// We add them here to make sure they are present.
	sbom = util.UpdateSBOMRepoMetadata(sbom, imgInspect.RepoTags, imgInspect.RepoDigests)

	return &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imgInspect.ID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   imageName,
			Labels: labels,
		},
		RepoTags:     imgInspect.RepoTags,
		RepoDigests:  imgInspect.RepoDigests,
		SizeBytes:    imgInspect.Size,
		OS:           imgInspect.Os,
		OSVersion:    imgInspect.OsVersion,
		Architecture: imgInspect.Architecture,
		Variant:      imgInspect.Variant,
		Layers:       layersFromDockerHistoryAndInspect(imageHistory, imgInspect),
		SBOM:         sbom,
	}, nil
}

// it has been observed that docker can return layers that are missing all metadata when inherited from a base container
func isInheritedLayer(layer image.HistoryResponseItem) bool {
	return layer.CreatedBy == "" && layer.Size == 0
}

func layersFromDockerHistoryAndInspect(history []image.HistoryResponseItem, inspect image.InspectResponse) []workloadmeta.ContainerImageLayer {
	var layers []workloadmeta.ContainerImageLayer

	// Loop through history and check how many layers should be assigned a corresponding docker inspect digest
	layersWithDigests := 0
	for _, layer := range history {
		if isInheritedLayer(layer) || layer.Size > 0 {
			layersWithDigests++
		}
	}

	// Layers that should be assigned a digest are determined by either of the following criteria:

	// A. The layer size > 0
	// B. The layer's size == 0 AND its CreatedBy field is empty, which means it's an inherited layer

	// This checks if the number of layers that should be assigned a digest exceeds the number of RootFS digests,
	// and prevents the agent from panicking from an index out of range error.

	shouldAssignDigests := true
	if layersWithDigests > len(inspect.RootFS.Layers) {
		log.Warn("Detected more history layers with possible digests than inspect layers, will not attempt to assign digests")
		shouldAssignDigests = false
	}

	// inspectIdx tracks the current RootFS layer ID index (in Docker, this corresponds to the Diff ID of a layer)
	// NOTE: Docker returns the RootFS layers in chronological order
	inspectIdx := 0

	// Docker returns the history layers in reverse-chronological order
	for i := len(history) - 1; i >= 0; i-- {
		created := time.Unix(history[i].Created, 0)
		isEmptyLayer := history[i].Size == 0
		isInheritedLayer := isInheritedLayer(history[i])

		digest := ""
		if shouldAssignDigests && (isInheritedLayer || !isEmptyLayer) {
			if isInheritedLayer {
				log.Debugf("detected an inherited layer for image ID: \"%s\", assigning it digest: \"%s\"", inspect.ID, inspect.RootFS.Layers[inspectIdx])
			}
			digest = inspect.RootFS.Layers[inspectIdx]
			inspectIdx++
		} else {
			// Fallback to previous behavior
			digest = history[i].ID
		}

		layer := workloadmeta.ContainerImageLayer{
			Digest:    digest,
			SizeBytes: history[i].Size,
			History: &v1.History{
				Created:    &created,
				CreatedBy:  history[i].CreatedBy,
				Comment:    history[i].Comment,
				EmptyLayer: isEmptyLayer,
			},
		}

		layers = append(layers, layer)
	}

	return layers
}
