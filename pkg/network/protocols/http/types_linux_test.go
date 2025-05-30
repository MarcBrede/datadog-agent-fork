// Code generated by genpost.go; DO NOT EDIT.

package http

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

func TestCgoAlignment_SslSock(t *testing.T) {
	ebpftest.TestCgoAlignment[SslSock](t)
}

func TestCgoAlignment_SslReadArgs(t *testing.T) {
	ebpftest.TestCgoAlignment[SslReadArgs](t)
}

func TestCgoAlignment_SslReadExArgs(t *testing.T) {
	ebpftest.TestCgoAlignment[SslReadExArgs](t)
}

func TestCgoAlignment_SslWriteArgs(t *testing.T) {
	ebpftest.TestCgoAlignment[SslWriteArgs](t)
}

func TestCgoAlignment_SslWriteExArgs(t *testing.T) {
	ebpftest.TestCgoAlignment[SslWriteExArgs](t)
}

func TestCgoAlignment_EbpfEvent(t *testing.T) {
	ebpftest.TestCgoAlignment[EbpfEvent](t)
}

func TestCgoAlignment_EbpfTx(t *testing.T) {
	ebpftest.TestCgoAlignment[EbpfTx](t)
}
