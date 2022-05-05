/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package sshtunnel

import (
	"errors"
	"os"

	"github.com/ansel1/merry"
	"golang.org/x/crypto/ssh"
)

var errExpectedFile = errors.New("expected file, got dir")

func PrivateKeyFile(file string) (ssh.AuthMethod, error) {
	info, err := os.Stat(file)
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}

	if info.IsDir() {
		return nil, errExpectedFile
	}

	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read private key file")
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, merry.Prepend(err, "failed to parse private key")
	}

	return ssh.PublicKeys(key), nil
}
