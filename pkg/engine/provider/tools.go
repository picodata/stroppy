/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package provider

import (
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	"github.com/ghodss/yaml"
	llog "github.com/sirupsen/logrus"
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

func DispatchTemplate(templatesConfig *ClusterConfigurations,
	flavor string) (template ClusterParameters, err error) {

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

func DeepCopyAddressMap(addressMap map[string]map[string]string) (copy map[string]map[string]string) {
	if addressMap == nil {
		return
	}

	copy = make(map[string]map[string]string, len(addressMap))
	for key, val := range addressMap {
		valLen := len(val)
		valCopy := make(map[string]string, valLen)
		for valKey, valVal := range val {
			valCopy[valKey] = valVal[:]
		}
		copy[key] = valCopy
	}
	return
}
