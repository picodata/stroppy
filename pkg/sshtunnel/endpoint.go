/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package sshtunnel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ansel1/merry"
)

type Endpoint struct {
	Host string
	Port int
	User string
}

func NewLocalEndpoint(port int, user string) *Endpoint {
	return &Endpoint{
		Host: "localhost",
		Port: port,
		User: user,
	}
}

func NewEndpoint(s string) (*Endpoint, error) {
	endpoint := &Endpoint{
		Host: s,
		Port: 0,
		User: "",
	}

	if parts := strings.Split(endpoint.Host, "@"); len(parts) > 1 {
		endpoint.User = parts[0]
		endpoint.Host = parts[1]
	}

	if parts := strings.Split(endpoint.Host, ":"); len(parts) > 1 {
		endpoint.Host = parts[0]

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, merry.Prepend(err, "failed to parse port value")
		}

		endpoint.Port = port
	}

	return endpoint, nil
}

func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}
