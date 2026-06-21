// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

// Command example is a runnable demo of picnic: it sends an HTTPS request out a
// chosen local network interface and prints the source address actually used,
// proving the bind took effect. For the godoc, copy-pasteable version see the
// ExampleName_BindDialer / ExampleName_BindListenConfig functions in
// example_test.go; this program exists to be run and watched live.
//
//	go run ./cmd/example                       # auto-pick an interface, GET https://github.com
//	go run ./cmd/example -iface en0            # bind to en0
//	go run ./cmd/example -list                 # list candidate interfaces and exit
//	go run ./cmd/example -url https://example.com -iface eth0
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/xmidt-org/picnic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	iface := flag.String("iface", "", "network interface to bind to (default: first usable)")
	url := flag.String("url", "https://github.com", "URL to GET")
	list := flag.Bool("list", false, "list candidate interfaces and exit")
	flag.Parse()

	if *list {
		return listInterfaces()
	}

	name := *iface
	if name == "" {
		picked, err := defaultInterface()
		if err != nil {
			return err
		}
		name = picked
		fmt.Printf("no -iface given; using %q\n", name)
	}

	// Step 1: configure a dialer to egress the chosen interface. This one call
	// is the entire integration — picnic sets the dialer's ControlContext.
	var dialer net.Dialer
	dialer.Timeout = 10 * time.Second
	if err := picnic.Name(name).BindDialer(&dialer); err != nil {
		return fmt.Errorf("binding to %q: %w", name, err)
	}

	// Step 2: build an http.Client that dials through it. net/http performs TLS
	// for https URLs on top of the socket picnic bound; picnic governs only the
	// underlying TCP connection. The DialContext wrapper is here just to print
	// the local address actually used, proving the bind took effect.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err == nil {
				fmt.Printf("dialed %s -> %s  (local %s, via %q)\n",
					addr, conn.RemoteAddr(), conn.LocalAddr(), name)
			}
			return conn, err
		},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	// Step 3: make the request. NewRequestWithContext + Do (rather than
	// client.Get) carries a context and avoids a gosec taint warning on a
	// variable URL.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, *url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("GET %s -> %s\n", *url, resp.Status)
	return nil
}

// defaultInterface returns the first up, non-loopback interface that has an
// address — a reasonable pick when the user does not name one.
func defaultInterface() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if addrs, _ := ifc.Addrs(); len(addrs) > 0 {
			return ifc.Name, nil
		}
	}
	return "", errors.New("no usable non-loopback interface found")
}

func listInterfaces() error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, ifc := range ifaces {
		addrs, _ := ifc.Addrs()
		fmt.Printf("%-16s %-28s %v\n", ifc.Name, ifc.Flags, addrs)
	}
	return nil
}
