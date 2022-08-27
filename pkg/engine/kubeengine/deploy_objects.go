/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/tools"
	"gopkg.in/yaml.v2"

	llog "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Accept path to manifest file, read it to bytes and try to cast into `objectType`
// that can one of the kubernetes Kind.
func (k8sEngine *Engine) ToEngineObject(
	objectName, manifestPath string,
	objectType interface{},
) error {
	var (
		err             error
		objectFileBytes []byte
	)

	if objectFileBytes, err = os.ReadFile(manifestPath); err != nil {
		return merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	if err = yaml.Unmarshal(objectFileBytes, objectType); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf(
				"failed to unmarshall object to kind %s configuration",
				reflect.TypeOf(objectType),
			),
		)
	}

	return nil
}

// Accept context and function for deploying some kubernetes kind
// plaease have attention that this function will not be waiting for successefull deploy.
func (k8sEngine *Engine) DeployObject(
	deployContext context.Context,
	objectApplyFunc func(*kubernetes.Clientset) error,
) error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	if clientSet, err = k8sEngine.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	if err = objectApplyFunc(clientSet); err != nil {
		return merry.Prepend(err, "failed to deploy")
	}

	return nil
}

func (k8sEngine *Engine) DeployAndWaitObject(
	deployContext context.Context,
	objectName, objectType string,
	objectApplyFunc func(*kubernetes.Clientset) error,
	objectDeleteFunc func(*kubernetes.Clientset) error,
) error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	llog.Infof(
		"Start deploying new %s namespace object %s with %d attempts",
		objectType,
		objectName,
		tools.RetryStandardRetryCount,
	)

	if clientSet, err = k8sEngine.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	if err = tools.Retry(
		fmt.Sprintf("deploy %s object %s", objectType, objectName),
		func() error {
			if err = objectApplyFunc(clientSet); err != nil {
				return merry.Prepend(
					err,
					fmt.Sprintf("error then waiting ready status for object %s", objectName),
				)
			}

			return nil
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime,
	); err != nil {
		if deleteErr := objectDeleteFunc(clientSet); deleteErr != nil {
			return merry.Prepend(
				deleteErr,
				fmt.Errorf(
					"failed to deploy and delete k8s object %s: %w", objectName, err,
				).Error())
		}

		return merry.Prepend(err, "failed to retry deploing k8s object")
	}

	llog.Infof("Object %s successefully deployed", objectName)

	return nil
}

// Returns `metav1.ApplyOptions` with default parametres
// `FieldManager` equals stroppy.
func (*Engine) GenerateDefaultMetav1() metav1.ApplyOptions {
	return metav1.ApplyOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		DryRun:       []string{},
		Force:        false,
		FieldManager: "stroppy",
	}
}

// Returns `metav1.DeleteOptions` with default parametres
// `PropagationPolicy` equals "backgrond".
func (*Engine) GenerateDefaultDeleteOtions() metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationBackground

	return metav1.DeleteOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		GracePeriodSeconds: new(int64),
		Preconditions:      &metav1.Preconditions{}, //nolint
		OrphanDependents:   new(bool),
		PropagationPolicy:  &propagationPolicy,
		DryRun:             []string{},
	}
}
