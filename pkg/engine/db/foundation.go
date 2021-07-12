package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"

	llog "github.com/sirupsen/logrus"

	"github.com/ansel1/merry"

	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const (
	foundationDbDirectory = "foundationdb"

	foundationClusterName       = "sample-cluster"
	foundationClusterClientName = "sample-cluster-client"
)

func CreateFoundationCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (fc Cluster) {
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

	var clientSet *kubernetes2.Clientset
	if clientSet, err = fc.k.GetClientSet(); err != nil {
		return merry.Prepend(err, "get client set")
	}

	var podList *v1.PodList
	podList, err = clientSet.CoreV1().
		Pods(kubernetes.ResourceDefaultNamespace).
		List(context.TODO(),
			metav1.ListOptions{
				TypeMeta: metav1.TypeMeta{},
			})
	if err != nil {
		return merry.Prepend(err, "get target pod list")
	}

	for i := 0; i < len(podList.Items); i++ {
		pod := podList.Items[i]
		llog.Debugf("examining pod: '%s'/'%s'", pod.Name, pod.GenerateName)

		if strings.HasPrefix(pod.Name, foundationClusterClientName) {
			llog.Infof("foundationdb main pod is '%s'", pod.Name)
			fc.clusterSpec.MainPod = &pod
		} else if strings.HasPrefix(pod.Name, foundationClusterName) {
			fc.clusterSpec.Pods = append(fc.clusterSpec.Pods, &pod)
		}
	}

	if fc.clusterSpec.MainPod == nil {
		return errors.New("foundation main pod does not exists")
	}

	if fc.clusterSpec.MainPod.Status.Phase != v1.PodRunning {

		fc.clusterSpec.MainPod, err = fc.k.WaitPod(clientSet, fc.clusterSpec.MainPod.Name,
			kubernetes.ResourceDefaultNamespace)
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

	// var textb []byte
	if textb, _err := session.CombinedOutput(fdbFixCommand); _err != nil {
		llog.Errorln(merry.Prependf(_err, "fix_client_version.sh failed with output `%s`", string(textb)))
		// return merry.Prependf(err, "fix_client_version.sh failed with output `%s`", string(textb))
	}
	llog.Debugf("fix_client_version.sh applyed successfully")

	// \todo: Прокидываем порт foundationdb на локальную машину

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

func (fc *foundationCluster) OpenPortForwarding() (_ error) {
	if err := fc.openPortForwarding(foundationClusterName, []string{":"}); err != nil {
		llog.Warnf("foundationdb failed to open port forwarding: %v", err)
	}

	return
}

func (fc *foundationCluster) GetStatus() (err error) {
	// already returns success
	return
}

func (fc *foundationCluster) GetSpecification() (spec ClusterSpec) {
	spec = fc.clusterSpec
	return
}
