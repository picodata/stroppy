/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package provider

import (
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

func LoadClusterTemplate(dir string) (*ClusterConfigurations, error) {
	templatesFilePath := filepath.Join(dir, ClusterTemplateFileName)

	data, err := ioutil.ReadFile(templatesFilePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yaml")
	}

	var templatesConfig ClusterConfigurations
	if err = yaml.Unmarshal(data, &templatesConfig); err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yaml")
	}

	llog.Traceln("reading templates.yaml: success")
	return &templatesConfig, nil
}

func DispatchTemplate(
	templatesConfig *ClusterConfigurations,
	flavor string,
) (template ClusterParameters, err error) {
	switch flavor {
	case "small":
		template = templatesConfig.Small
	case "standard":
		template = templatesConfig.Standard
	case "large":
		template = templatesConfig.Large
	case "xlarge":
		template = templatesConfig.XLarge
	case "xxlarge":
		template = templatesConfig.XXLarge
	default:
		err = merry.Wrap(ErrChooseConfig)
	}

	return
}
