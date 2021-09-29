/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package engine

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	llog "github.com/sirupsen/logrus"
)

// IsLocalPortOpen
// проверить доступность порта на localhost
func IsLocalPortOpen(port int) bool {
	address := "localhost:" + strconv.Itoa(port)
	conn, err := net.Listen("tcp", address)
	if err != nil {
		llog.Errorf("port %v at localhost is not available \n", port)
		return false
	}

	if err = conn.Close(); err != nil {
		llog.Errorf("failed to close port %v at localhost after check\n", port)
		return false
	}
	return true
}

// IsRemotePortOpen - проверить доступность порта на удаленной машине кластера
func IsRemotePortOpen(hostname string, port int) bool {
	address := hostname + ":" + strconv.Itoa(port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		llog.Errorf("port %d at '%s' is not available: %v \n", port, address, err)
		return false
	}

	_ = conn.Close()
	return true
}

// FilterPipe вывводит буфер стандартного вывода в отдельном потоке
func FilterPipe(reader io.Reader, expectedWaitChan bool) (waitChannel chan struct{}) {
	if expectedWaitChan {
		waitChannel = make(chan struct{})
	}

	bufReader := bufio.NewReader(reader)
	go func() {
		printOutput := llog.GetLevel() == llog.DebugLevel
		for {
			str, err := bufReader.ReadString('\n')
			if err != nil {
				break
			}
			if printOutput {
				llog.Debugln(str)
			}
		}

		if expectedWaitChan {
			fmt.Printf("\n\n\n\nYeah, baby! chan id is %v\n\n\n\n\n", waitChannel)
			waitChannel <- struct{}{}
			close(waitChannel)
		}
	}()

	return
}

func IsFileExists(workingDirectory string, file string) bool {
	privateKeyPath := filepath.Join(workingDirectory, file)

	if _, err := os.Stat(privateKeyPath); err != nil {
		if os.IsNotExist(err) {
			llog.Errorf("file %v not found. Create it, please.\n", file)
			return false
		}
		llog.Errorf("failed to find file %v: %v\n", file, err)
		return false
	}
	return true
}
