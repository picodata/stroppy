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

func Retry(tag string, f func() error, retryCount, sleepTimeout int) (err error) {
	for i := 0; i < retryCount; i++ {
		err = f()
		if err == nil {
			return
		}
		llog.Errorf("Retry '%s', run %d/%d: %v", tag, i, retryCount, err)

		if sleepTimeout > 0 {
			time.Sleep(time.Duration(sleepTimeout) * time.Second)
		}
	}

	return
}
