package kubernetes

import (
	"fmt"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func (k *Kubernetes) executeCommand(text string) (err error) {
	var commandSessionObject engineSsh.Session
	if commandSessionObject, err = k.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	if result, err := commandSessionObject.CombinedOutput(text); err != nil {
		// выводим, чтобы было проще диагностировать
		llog.Errorln(string(result))
		return merry.Prepend(err, "output collect failed")
	}

	return
}

func (k *Kubernetes) Execute(text string) (err error) {
	err = k.executeCommand(text)
	return
}

func (k *Kubernetes) ExecuteF(text string, args ...interface{}) (err error) {
	err = k.executeCommand(fmt.Sprintf(text, args...))
	return
}
