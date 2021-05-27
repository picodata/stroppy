package kubernetes

import (
	"github.com/ansel1/merry"
	"k8s.io/client-go/tools/clientcmd"
)

func editClusterURL(url string) error {
	kubeConfigPath := "benchmark/deploy/config"
	kubeConfig, err := clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return merry.Prepend(err, "failed to load kubeconfig")
	}
	// меняем значение адреса кластера внутри kubeconfig
	kubeConfig.Clusters["cluster.local"].Server = url

	err = clientcmd.WriteToFile(*kubeConfig, kubeConfigPath)
	if err != nil {
		return merry.Prepend(err, "failed to write kubeconfig")
	}

	return nil
}
