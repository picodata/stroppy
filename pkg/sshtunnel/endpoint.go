/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package sshtunnel

import (
	"fmt"

	llog "github.com/sirupsen/logrus"
)

type Endpoint struct {
	Host string
	Port int
	User string
}

func NewLocalEndpoint(port int, user string) *Endpoint {
	llog.Tracef(
		"New local endpoint with host: localhost, port: %d, user: %s",
		port,
		user,
	)

	return &Endpoint{
		Host: "localhost",
		Port: port,
		User: user,
	}
}

func NewRemoteEndpoint(hostname string, port int, user string) *Endpoint {
	endpoint := &Endpoint{
		Host: hostname,
		Port: port,
		User: user,
	}

	llog.Tracef(
		"New remote endpoint with host: %s, port: %d, user: %s",
		endpoint.Host,
		endpoint.Port,
		endpoint.User,
	)

	return endpoint
}

func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}
