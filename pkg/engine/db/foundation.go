/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"fmt"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"

	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

const (
	foundationClusterName       = "sample-cluster"
	foundationClusterClientName = "sample-cluster-client"
)

func createFoundationCluster(
	sshClient engineSsh.Client,
	k8s *kubernetes.Kubernetes,
	shellState *state.State,
) Cluster {
	return &foundationCluster{
		commonCluster: createCommonCluster(
			sshClient,
			k8s,
			shellState,
		),
	}
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

func (fc *foundationCluster) Deploy(
	_ *kubernetes.Kubernetes,
	shellState *state.State,
) error {
	var err error

	if err = fc.deploy(shellState); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	if err = fc.examineCluster("FoundationDB",
		kubeengine.ResourceDefaultNamespace,
		foundationClusterClientName,
		foundationClusterName,
	); err != nil {
		return merry.Prepend(err, "failed to examine FoundationDB cluster")
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

	llog.Debug("fix_client_version.sh applied successfully")

	// \todo: Прокидываем порт foundationdb на локальную машину
	if err = fc.openPortForwarding(foundationClusterName, []string{":"}); err != nil {
		llog.Warnf("foundationdb failed to open port forwarding: %v", err)
	}

	if fc.k.StroppyPod != nil {
		var contName string
		if contName, err = fc.k.StroppyPod.ContainerName(0); err != nil {
			return merry.Prepend(err, "failed to get stroppy container name")
		}

		sourceConfigPath := fmt.Sprintf("%s/%s://var/dynamic-conf/fdb.cluster",
			foundationClusterClientName, fc.clusterSpec.MainPod.Name)
		destinationConfigPath := fmt.Sprintf("stroppy-client/%s://bin", contName)
		if _err := fc.k.Engine.CopyFileFromPodToPod(sourceConfigPath, destinationConfigPath); _err != nil {
			llog.Errorln(merry.Prepend(_err, "fdb.cluster file copying"))
		}
	}

	return nil
}

func (fc *foundationCluster) GetSpecification() ClusterSpec {
	return fc.clusterSpec
}
