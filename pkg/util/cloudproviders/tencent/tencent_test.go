// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tencent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetInstanceID(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"tencent"})

	expected := "ins-nad6bga0"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	val, err := GetInstanceID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expected, val)
	assert.Equal(t, lastRequest.URL.Path, "/meta-data/instance-id")
}

func TestGetHostAliases(t *testing.T) {
	cfg := configmock.New(t)
	ctx := context.Background()
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"tencent"})

	expected := "ins-nad6bga0"
	var lastRequest *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, expected)
		lastRequest = r
	}))
	defer ts.Close()
	metadataURL = ts.URL

	aliases, err := GetHostAliases(ctx)
	assert.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, expected, aliases[0])
	assert.Equal(t, lastRequest.URL.Path, "/meta-data/instance-id")
}

func TestGetNTPHosts(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("cloud_provider_metadata", []string{"tencent"})

	ctx := context.Background()
	expectedHosts := []string{"ntpupdate.tencentyun.com"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "test")
	}))
	defer ts.Close()

	metadataURL = ts.URL
	actualHosts := GetNTPHosts(ctx)

	assert.Equal(t, expectedHosts, actualHosts)
}
