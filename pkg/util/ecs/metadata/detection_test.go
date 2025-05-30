// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
)

func TestLocateECSHTTP(t *testing.T) {
	assert := assert.New(t)

	ecsinterface, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/", "./v1/testdata/commands.json"),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	cfg := mock.New(t)
	cfg.SetWithoutSource("ecs_agent_url", ts.URL)

	_, err = newAutodetectedClientV1()
	require.NoError(t, err)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/", r.URL.Path)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
}

func TestLocateECSHTTPFail(t *testing.T) {
	assert := assert.New(t)

	ecsinterface, err := testutil.NewDummyECS()
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	cfg := mock.New(t)
	cfg.SetDefault("ecs_agent_url", ts.URL)

	_, err = newAutodetectedClientV1()
	require.Error(t, err)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/", r.URL.Path)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
}

func TestGetAgentV1ContainerURLs(t *testing.T) {
	env.SetFeatures(t, env.Docker)

	cfg := mock.New(t)
	ctx := context.Background()
	cfg.SetWithoutSource("ecs_agent_container_name", "ecs-agent-custom")
	defer cfg.SetWithoutSource("ecs_agent_container_name", "ecs-agent")

	// Setting mocked data in cache
	nets := make(map[string]*network.EndpointSettings)
	nets["bridge"] = &network.EndpointSettings{IPAddress: "172.17.0.2"}
	nets["foo"] = &network.EndpointSettings{IPAddress: "172.17.0.3"}

	co := container.InspectResponse{
		Config: &container.Config{
			Hostname: "ip-172-29-167-5",
		},
		ContainerJSONBase: &container.ContainerJSONBase{},
		NetworkSettings: &container.NetworkSettings{
			Networks: nets,
		},
	}
	docker.EnableTestingMode()
	cacheKey := docker.GetInspectCacheKey("ecs-agent-custom", false)
	cache.Cache.Set(cacheKey, co, 10*time.Second)

	agentURLS, err := getAgentV1ContainerURLs(ctx)
	assert.NoError(t, err)
	require.Len(t, agentURLS, 3)
	assert.Contains(t, agentURLS, "http://172.17.0.2:51678/")
	assert.Contains(t, agentURLS, "http://172.17.0.3:51678/")
	assert.Equal(t, "http://ip-172-29-167-5:51678/", agentURLS[2])
}

func TestIsMetadataV4Available(t *testing.T) {
	ok, err := IsMetadataV4Available("")
	assert.NotNil(t, err)
	assert.False(t, ok)

	ok, err = IsMetadataV4Available("1.0.0")
	assert.NoError(t, err)
	assert.False(t, ok)

	ok, err = IsMetadataV4Available("1.80.0")
	assert.NoError(t, err)
	assert.True(t, ok)
}
