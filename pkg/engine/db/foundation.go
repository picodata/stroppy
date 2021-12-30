/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"fmt"
	"path/filepath"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"

	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"

	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

const (
	foundationDbDirectory = "foundationdb"

	foundationClusterName       = "sample-cluster"
	foundationClusterClientName = "sample-cluster-client"
)

func createFoundationCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string) (fc Cluster) {
	fc = &foundationCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, foundationDbDirectory),
			foundationDbDirectory,
			dbURL,
			0,
			false,
		),
	}
	return
}

type foundationCluster struct {
	*commonCluster
}

func (fc *foundationCluster) Connect() (cluster interface{}, err error) {
	if fc.DBUrl == "" {
		fc.DBUrl = "fdb.cluster"
	}

	cluster, err = cluster2.NewFoundationCluster(fc.DBUrl)
	return
}

func (fc *foundationCluster) Deploy() (err error) {
	if err = fc.deploy(); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	err = fc.examineCluster("FoundationDB",
		kubeengine.ResourceDefaultNamespace,
		foundationClusterClientName,
		foundationClusterName)
	if err != nil {
		return
	}
	llog.Infof("Now perform additional foundation deployment steps")

	var session engineSsh.Session
	if session, err = fc.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "fix_client_version session")
	}

	const fdbFixCommand = "chmod +x foundationdb/fix_client_version.sh && ./foundationdb/fix_client_version.sh"

	var textb []byte
	if textb, err = session.CombinedOutput(fdbFixCommand); err != nil {
		return merry.Prependf(err, "fix_client_version.sh failed with output `%s`", string(textb))
	}
	llog.Debugf("fix_client_version.sh applyed successfully")

	// \todo: Прокидываем порт foundationdb на локальную машину
	if err := fc.openPortForwarding(foundationClusterName, []string{":"}); err != nil {
		llog.Warnf("foundationdb failed to open port forwarding: %v", err)
	}

	if fc.k.StroppyPod != nil {
		var contName string
		if contName, err = fc.k.StroppyPod.ContainerName(0); err != nil {
			return
		}

		sourceConfigPath := fmt.Sprintf("%s/%s://var/dynamic-conf/fdb.cluster",
			foundationClusterClientName, fc.clusterSpec.MainPod.Name)
		destinationConfigPath := fmt.Sprintf("stroppy-client/%s://bin", contName)
		if _err := fc.k.Engine.CopyFileFromPodToPod(sourceConfigPath, destinationConfigPath); _err != nil {
			llog.Errorln(merry.Prepend(_err, "fdb.cluster file copying"))
		}
	}

	return
}

func (fc *foundationCluster) GetSpecification() (spec ClusterSpec) {
	spec = fc.clusterSpec
	return
}
