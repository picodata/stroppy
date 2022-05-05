/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package tools

import (
	"os"
	"path/filepath"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

const (
	RetryStandardRetryCount = 5

	RetryStandardWaitingTime = 5
	RetryStandardNoWaitTime  = 0
)

// Retry - выполнить переповтор функции с возвратом ошибки.
func Retry(tag string, fClos func() error, retryCount, sleepTimeout int) (err error) {
	for i := 0; i < retryCount; i++ {
		err = fClos()
		if err == nil {
			return nil
		}

		llog.Warnf("Retry '%s', run %d/%d: %v", tag, i, retryCount, err)

		if sleepTimeout > 0 {
			time.Sleep(time.Duration(sleepTimeout) * time.Second)
		}
	}

	return err
}

func RemovePathList(list []string, rootDir string) {
	var err error

	for _, file := range list {
		path := filepath.Join(rootDir, file)
		if err = os.RemoveAll(path); err != nil {
			llog.Warnf("delete file: %v", merry.Prepend(err, path))
		}
	}
}
