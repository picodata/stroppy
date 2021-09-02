package kubeengine

import (
	"fmt"

	"github.com/ansel1/merry"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
)

func (e *Engine) ExecuteCommand(text string) (err error) {
	var commandSessionObject engineSsh.Session
	if commandSessionObject, err = e.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	if result, err := commandSessionObject.CombinedOutput(text); err != nil {
		return merry.Prependf(err, "command exec failed with output `%s`", string(result))
	}

	return
}

func (e *Engine) Execute(text string) (err error) {
	err = e.ExecuteCommand(text)
	return
}

func (e *Engine) ExecuteF(text string, args ...interface{}) (err error) {
	err = e.ExecuteCommand(fmt.Sprintf(text, args...))
	return
}
