/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package ssh

import (
	"errors"
	"io"
	"os/exec"
	"sync"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func createLocalClient(wd string) (cc Client, err error) {
	c := &localClient{
		wd: wd,
	}

	llog.Infof("local shell client created")
	cc = c
	return
}

type localClient struct {
	wd string
}

func (dc *localClient) GetNewSession() (session Session, _ error) {
	session = createLocalSession(dc.wd)
	return
}

func (dc *localClient) GetPrivateKeyInfo() (string, string) {
	return "no-object", "/no/object"
}

// createLocalSession создает фиктивную (локальную) сессию
func createLocalSession(wd string) (session *localSession) {
	session = &localSession{
		wd:      wd,
		cmdLock: sync.Mutex{},
	}
	return
}

type localSession struct {
	wd   string
	text string

	cmd     *exec.Cmd
	cmdLock sync.Mutex
}

func (ds *localSession) CombinedOutput(text string) (output []byte, err error) {
	ds.cmdLock.Lock()
	defer ds.cmdLock.Unlock()

	_ = ds.close()

	ds.cmd = exec.Command(text)
	ds.cmd.Dir = ds.wd

	if err = ds.cmd.Start(); err != nil {
		err = merry.Prepend(err, "failed to start")
		return
	}

	output, err = ds.cmd.CombinedOutput()
	return
}

func (ds *localSession) StdoutPipe() (stdout io.Reader, err error) {
	ds.cmdLock.Lock()
	defer ds.cmdLock.Unlock()

	if ds.cmd == nil {
		err = errors.New("no process")
		return
	}

	stdout, err = ds.cmd.StdoutPipe()
	return
}

func (ds *localSession) Close() (_ error) {
	ds.cmdLock.Lock()
	defer ds.cmdLock.Unlock()

	_ = ds.close()
	return
}

func (ds *localSession) close() (_ error) {
	if ds.cmd == nil {
		return
	}

	if err := ds.cmd.Process.Kill(); err != nil {
		llog.Warnf("failed to kill process '%s', pid %d on directory '%s': %v",
			ds.text, ds.cmd.Process.Pid, ds.wd, err)
	}

	return
}
