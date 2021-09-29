/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package ssh

import (
	"bytes"
	"fmt"
	"io"

	llog "github.com/sirupsen/logrus"
)

func createDummyClient() (cc Client, _ error) {
	cc = &dummyClient{}

	llog.Debugf("dummy shell client created")
	return
}

type dummyClient struct{}

func (dc *dummyClient) GetNewSession() (session Session, _ error) {
	session = &dummySession{}
	llog.Debug("new dummy session successfully created")
	return
}

func (dc *dummyClient) GetPrivateKeyInfo() (_, _ string) {
	return
}

type dummySession struct{}

func (ds *dummySession) CombinedOutput(text string) (output []byte, _ error) {
	output = []byte(fmt.Sprintf("dummy output for '%s' command", text))
	return
}

func (ds *dummySession) StdoutPipe() (stdout io.Reader, _ error) {
	m := "dummy session stdout pipe content"
	stdout = bytes.NewBufferString(m)
	return
}

func (ds *dummySession) Close() (_ error) {
	return
}
