/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	"sync"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine/db"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gopkg.in/inf.v0"
)

type Payload interface {
	Pay(string) error
	Pop(string) error
	Check(*inf.Dec) (*inf.Dec, error)
	UpdateSettings(*config.DatabaseSettings)
	StartStatisticsCollect(statInterval time.Duration) error
	Connect() error
}

type BasePayload struct {
	// \todo: Имеем две сущности описывающие кластер базы данных - произвести рефакторинг
	cluster db.Cluster
	Cluster CustomTxTransfer

	config     *config.DatabaseSettings
	configLock sync.Mutex

	chaos          chaos.Controller
	chaosParameter string

	oracle  *database.Oracle
	payFunc func(settings *config.DatabaseSettings, cluster CustomTxTransfer, oracle *database.Oracle) (*PayStats, error)
}

func CreatePayload(
	cluster db.Cluster,
	settings *config.Settings,
	chaosController chaos.Controller,
) (Payload, error) {
	basePayload := &BasePayload{
		cluster:        cluster,
		Cluster:        nil,
		config:         settings.DatabaseSettings,
		configLock:     sync.Mutex{},
		chaos:          chaosController,
		chaosParameter: settings.ChaosParameter,
		oracle:         &database.Oracle{},
		payFunc:        nil,
	}

	llog.Debugf("DatabaseSettings: DBType: %s, workers: %d, Zipfian: %v, Oracle: %v, Check: %v, "+
		"DBURL: %s, UseCustomTx: %v, BanRangeMultiplier: %v, StatInterval: %v, "+
		"ConnectPoolSize: %d, Sharded: %v",
		settings.DatabaseSettings.DBType,
		settings.DatabaseSettings.Workers,
		settings.DatabaseSettings.Zipfian,
		settings.DatabaseSettings.Oracle,
		settings.DatabaseSettings.Check,
		settings.DatabaseSettings.DBURL,
		settings.DatabaseSettings.UseCustomTx,
		settings.DatabaseSettings.BanRangeMultiplier,
		settings.DatabaseSettings.StatInterval,
		settings.DatabaseSettings.ConnectPoolSize,
		settings.DatabaseSettings.Sharded,
	)

	if basePayload.config.Oracle {
		predictableCluster, ok := basePayload.Cluster.(database.PredictableCluster)
		if !ok {
			return nil, merry.Errorf(
				"Oracle is not supported for %s cluster",
				basePayload.config.DBType,
			)
		}

		basePayload.oracle = new(database.Oracle)

		basePayload.oracle.Init(predictableCluster)
	}

	if basePayload.config.UseCustomTx {
		basePayload.payFunc = payCustomTx
	} else {
		basePayload.payFunc = payBuiltinTx
	}

	llog.Infof(
		"Payload object constructed for database '%s', url '%s'",
		basePayload.config.DBType,
		basePayload.config.DBURL,
	)

	return basePayload, nil
}

func (p *BasePayload) UpdateSettings(newConfig *config.DatabaseSettings) {
	p.configLock.Lock()
	defer p.configLock.Unlock()

	unpConfig := *newConfig
	p.config = &unpConfig
}

func (p *BasePayload) StartStatisticsCollect(statInterval time.Duration) (err error) {
	if err = p.Cluster.StartStatisticsCollect(statInterval); err != nil {
		return merry.Errorf("failed to get statistic for %v cluster: %v", p.config.DBType, err)
	}

	return
}

func (p *BasePayload) Connect() (err error) {
	// \todo: необходим большой рефакторинг
	var c interface{}
	if c, err = p.cluster.Connect(); err != nil {
		return
	}

	p.Cluster = c.(CustomTxTransfer)
	return
}
