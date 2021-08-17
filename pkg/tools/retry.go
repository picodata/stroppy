package tools

import (
	"time"

	llog "github.com/sirupsen/logrus"
)

const (
	RetryStandardRetryCount = 5

	RetryStandardWaitingTime = 5
	RetryStandardNoWaitTime  = 0
)

// Retry - выполнить переповтор функции с возвратом ошибки
func Retry(tag string, fClos func() error, retryCount, sleepTimeout int) (err error) {
	for i := 0; i < retryCount; i++ {
		err = fClos()
		if err == nil {
			return
		}
		llog.Warnf("Retry '%s', run %d/%d: %v", tag, i, retryCount, err)

		if sleepTimeout > 0 {
			time.Sleep(time.Duration(sleepTimeout) * time.Second)
		}
	}

	return
}

// RetryWithResult - выполнить переповтор фукнции с возвратом результата и ошибки
func RetryWithResult(tag string, fClos func() (result interface{}, err error), retryCount, sleepTimeout int) (result interface{}, err error) {
	for i := 0; i < retryCount; i++ {
		result, err = fClos()
		if err == nil {
			return
		}
		llog.Warnf("Retry '%s', run %d/%d: %v", tag, i, retryCount, err)

		if sleepTimeout > 0 {
			time.Sleep(time.Duration(sleepTimeout) * time.Second)
		}
	}

	return
}
