// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package amqp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	httpUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	// User is the user to use for authentication
	User = "guest"
	// Pass is the password to use for authentication
	Pass = "guest"
)

type encryptionPoliciesMap map[bool]string
type regexGeneratorsMap map[bool]func(testing.TB, string) *regexp.Regexp

var (
	encryptionPolicies      = encryptionPoliciesMap{protocolsUtils.TLSDisabled: "plaintext", protocolsUtils.TLSEnabled: "tls"}
	startupRegexpGenerators = regexGeneratorsMap{
		protocolsUtils.TLSDisabled: getPlaintextRegexp,
		protocolsUtils.TLSEnabled:  getTLSRegexp,
	}
)

// RunServer runs an AMQP server in a docker container.
func RunServer(t testing.TB, serverAddr, serverPort string, enableTLS bool) error {
	t.Helper()

	env := getServerEnv(t, serverAddr, serverPort, enableTLS)
	startupRegexp := startupRegexpGenerators[enableTLS](t, serverPort)

	dir, _ := httpUtils.CurDir()

	scanner, err := globalutils.NewScanner(startupRegexp, globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")
	dockerCfg := dockerutils.NewComposeConfig("amqp",
		dockerutils.DefaultTimeout,
		dockerutils.DefaultRetries,
		scanner,
		env,
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}

// getServerEnv returns the environment to configure the amqp server
func getServerEnv(t testing.TB, serverAddr, serverPort string, withTLS bool) []string {
	t.Helper()

	cert, _, err := httpUtils.GetCertsPaths()
	require.NoError(t, err)
	certsDir := filepath.Dir(cert)

	// The certificates are bind-mounted in the container. They
	// inherit permissions from the host, so we ensure the permissions
	// allow RabbitMQ to read the certificate/key pair.
	curDir, _ := httpUtils.CurDir()
	require.NoError(t, os.Chmod(curDir+"/testdata/tls.conf", 0644))
	require.NoError(t, os.Chmod(curDir+"/testdata/plaintext.conf", 0644))
	require.NoError(t, os.Chmod(certsDir+"/server.key", 0644))
	require.NoError(t, os.Chmod(certsDir+"/cert.pem.0", 0644))

	return []string{
		"AMQP_ADDR=" + serverAddr,
		"AMQP_PORT=" + serverPort,
		"USER=" + User,
		"PASS=" + Pass,
		"CERTS_PATH=" + certsDir,
		"ENCRYPTION_POLICY=" + encryptionPolicies[withTLS],
	}
}

// getPlaintextRegexp the startup regexp to check for proper
// initialization of the plaintext AMQP server.
func getPlaintextRegexp(t testing.TB, serverPort string) *regexp.Regexp {
	t.Helper()

	return regexp.MustCompile(fmt.Sprintf(".*started TCP listener on .*%s.*", serverPort))
}

// getTLSRegexp returns the startup regexp to check for proper
// initialization of the TLS-enabled AMQP server.
func getTLSRegexp(t testing.TB, serverPort string) *regexp.Regexp {
	t.Helper()

	return regexp.MustCompile(fmt.Sprintf(".*started TLS \\(SSL\\) listener on .*%s.*", serverPort))
}
