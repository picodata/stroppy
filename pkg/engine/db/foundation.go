package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	llog "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"

	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const (
	foundationDbDirectory = "foundationdb"

	foundationClusterName       = "sample-cluster"
	foundationClusterClientName = "sample-cluster-client"
)

func createFoundationCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (fc Cluster) {
	fc = &foundationCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, foundationDbDirectory),
			foundationDbDirectory,
		),
	}
	return
}

type foundationCluster struct {
	*commonCluster
}

func (fc *foundationCluster) Deploy() (err error) {
	if err = fc.deploy(); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	var pods []v1.Pod
	if pods, err = fc.k.ListPods(kubernetes.ResourceDefaultNamespace); err != nil {
		err = merry.Prepend(err, "list foundation pods")
		return
	}

	printPodContainers := func(pod *v1.Pod) {
		for _, c := range pod.Spec.Containers {
			llog.Debugf("\tfound (%s, `%s`, '%s') container in pod '%s'",
				c.Name, strings.Join(c.Args, " "), strings.Join(c.Command, " "), pod.Name)
		}
		llog.Debug("\t---------------------\n\n")
	}
	for i := 0; i < len(pods); i++ {
		pPod := &pods[i]

		llog.Debugf("examining pod: '%s'/'%s'", pPod.Name, pPod.GenerateName)
		if strings.HasPrefix(pPod.Name, foundationClusterClientName) {
			llog.Infof("foundationdb main pod is '%s'", pPod.Name)
			printPodContainers(pPod)
			fc.clusterSpec.MainPod = pPod
		} else if strings.HasPrefix(pPod.Name, foundationClusterName) {
			fc.clusterSpec.Pods = append(fc.clusterSpec.Pods, pPod)
			printPodContainers(pPod)
		}
	}

	if fc.clusterSpec.MainPod == nil {
		return errors.New("foundation main pod does not exists")
	}

	if fc.clusterSpec.MainPod.Status.Phase != v1.PodRunning {
		fc.clusterSpec.MainPod, err = fc.k.WaitPod(fc.clusterSpec.MainPod.Name,
			kubernetes.ResourceDefaultNamespace,
			kubernetes.PodWaitingWaitCreation,
			kubernetes.PodWaitingTime10Minutes)
		if err != nil {
			return merry.Prepend(err, "foundation pod wait")
		}
	}
	llog.Debugf("foundation main pod '%s' in status '%s', okay",
		fc.clusterSpec.MainPod.Name, fc.clusterSpec.MainPod.Status.Phase)

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
		sourceConfigPath := fmt.Sprintf("%s/%s:///var/dynamic-conf/fdb.cluster",
			foundationClusterClientName, fc.clusterSpec.MainPod.Name)
		destinationConfigPath := fmt.Sprintf("stroppy-client/%s://bin", fc.k.StroppyPod.Spec.Containers[0].Name)
		if _err := fc.k.CopyFileFromPodToPod(sourceConfigPath, destinationConfigPath); _err != nil {
			llog.Errorln(merry.Prepend(_err, "fdb.cluster file copying"))
		}
	}

	return
}

func (fc *foundationCluster) GetSpecification() (spec ClusterSpec) {
	spec = fc.clusterSpec
	return
}
