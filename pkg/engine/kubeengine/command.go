/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"fmt"
	"io"

	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"

	"github.com/ansel1/merry"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
)

func (e *Engine) ExecuteCommand(text string) (err error) {
	var commandSessionObject engineSsh.Session
	if commandSessionObject, err = e.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	llog.Debugf("launch command `%s", text)
	if result, err := commandSessionObject.CombinedOutput(text); err != nil {
		return merry.Prependf(err, "command exec failed with output `%s`", string(result))
	}
	llog.Debugf("`%s` command complete", text)

	return
}

func (e *Engine) DebugCommand(text string, waitComplete bool) (err error) {
	var sshSession engineSsh.Session
	if sshSession, err = e.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	var stdout io.Reader
	if stdout, err = sshSession.StdoutPipe(); err != nil {
		return merry.Prepend(err, "failed creating command stdout pipe")
	}

	waitCh := engine.FilterPipe(stdout, waitComplete)

	llog.Debugf("debug command `%s", text)

	var textOut []byte
	if textOut, err = sshSession.CombinedOutput(text); err != nil {
		return merry.Prependf(err, "command execution failed, return text `%s`", string(textOut))
	}

	if waitComplete {
		<-waitCh
	}
	llog.Debugf("`%s` command debug complete", text)

	return
}

func (e *Engine) ExecuteF(text string, args ...interface{}) (err error) {
	err = e.ExecuteCommand(fmt.Sprintf(text, args...))
	return
}
