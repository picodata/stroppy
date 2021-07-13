package ssh

import (
	"errors"
	"io"
	"os/exec"
	"sync"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func createDummyClient(wd string) (cc Client, err error) {
	c := &dummyClient{
		wd: wd,
	}

	llog.Infof("dummy (local) shell client created")
	cc = c
	return
}

type dummyClient struct {
	wd string
}

func (dc *dummyClient) GetNewSession() (session Session, _ error) {
	session = createDummySession(dc.wd)
	return
}

func (dc *dummyClient) GetPrivateKeyInfo() (string, string) {
	return "no-object", "/no/object"
}

// createDummySession создает фиктивную (локальную) сессию
func createDummySession(wd string) (session *dummySession) {
	session = &dummySession{
		wd:      wd,
		cmdLock: sync.Mutex{},
	}
	return
}

type dummySession struct {
	wd   string
	text string

	cmd     *exec.Cmd
	cmdLock sync.Mutex
}

func (ds *dummySession) CombinedOutput(text string) (output []byte, err error) {
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

func (ds *dummySession) StdoutPipe() (stdout io.Reader, err error) {
	ds.cmdLock.Lock()
	defer ds.cmdLock.Unlock()

	if ds.cmd == nil {
		err = errors.New("no process")
		return
	}

	stdout, err = ds.cmd.StdoutPipe()
	return
}

func (ds *dummySession) Close() (_ error) {
	ds.cmdLock.Lock()
	defer ds.cmdLock.Unlock()

	_ = ds.close()
	return
}

func (ds *dummySession) close() (_ error) {
	if ds.cmd == nil {
		return
	}

	if err := ds.cmd.Process.Kill(); err != nil {
		llog.Warnf("failed to kill process '%s', pid %d on directory '%s': %v",
			ds.text, ds.cmd.Process.Pid, ds.wd, err)
	}

	return
}
