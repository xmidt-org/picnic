// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/xmidt-org/picnic"
)

// Send an HTTPS request out a specific interface. picnic governs only the TCP
// socket; net/http layers TLS for the https URL on top of it.
//
// (No Output: line — this reaches the network and depends on the host's
// interfaces, so it is compiled as documentation but not run by `go test`.)
func ExampleName_BindDialer() {
	// Binding the dialer is the entire integration.
	var dialer net.Dialer
	if err := picnic.Name("eth0").BindDialer(&dialer); err != nil {
		log.Fatal(err)
	}

	client := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, "https://github.com", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)
}

// Open a UDP socket bound to a specific interface for a QUIC / HTTP-3 /
// WebTransport stack. The resulting net.PacketConn is what you hand to, e.g.,
// quic-go's quic.Transport{Conn: pc}.
func ExampleName_BindListenConfig() {
	var lc net.ListenConfig
	if err := picnic.Name("eth0").BindListenConfig(&lc); err != nil {
		log.Fatal(err)
	}

	pc, err := lc.ListenPacket(context.Background(), "udp4", ":0")
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	// Hand pc to a QUIC stack to run HTTP-3 / WebTransport out this interface.
	_ = pc
}
