/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"context"

	"github.com/ansel1/merry"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
)

func (e *Engine) ListPods(namespace string) (pods []v1.Pod, err error) {
	var clientSet *kubernetes2.Clientset

	if clientSet, err = e.GetClientSet(); err != nil {
		err = merry.Prepend(err, "get client set")

		return
	}

	var podList *v1.PodList

	podList, err = clientSet.CoreV1().
		Pods(namespace).
		List(context.TODO(),
			metav1.ListOptions{
				TypeMeta: metav1.TypeMeta{},
			})
	if err != nil {
		err = merry.Prepend(err, "get pod list")

		return
	}

	pods = podList.Items

	return
}
