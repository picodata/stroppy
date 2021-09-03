package db

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/tools"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	mongoDirectory = "mongodb"

	mongoOperatorName     = "mongodb-kubernetes-operator"
	mongoClusterNamespace = "mongodbcommunity"
)

func createMongoCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string) (mongo Cluster) {
	mongo = &mongoCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, mongoDirectory),
			mongoDirectory,
			dbURL,
		),
	}
	return
}

type mongoCluster struct {
	*commonCluster
}

func (mongo *mongoCluster) Connect() (cluster interface{}, err error) {
	// подключение к локально развернутому mongo без реплики
	if mongo.DBUrl == "" {
		mongo.DBUrl = "mongodb://127.0.0.1:30001"
	}

	cluster, err = cluster2.NewMongoDBCluster(mongo.DBUrl, 64)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to  mongo cluster")
	}
	return
}

func (mongo *mongoCluster) Deploy() (err error) {
	if err = mongo.AddPersistentVolumesClaims(); err != nil {
		return merry.Prepend(err, "failed to add pvc for mongodb")
	}
	if err = mongo.deploy(); err != nil {
		return merry.Prepend(err, "base deployment step")
	}

	err = mongo.examineCluster("MongoDB",
		mongoClusterNamespace,
		mongoOperatorName,
		"")
	if err != nil {
		return
	}

	return
}

func (mongo *mongoCluster) GetSpecification() (spec ClusterSpec) {
	return
}

func (mongo *mongoCluster) AddPersistentVolumesClaims() error {
	llog.Infoln("Starting of adding persistent volume claims for mongodb")

	clientSet, err := mongo.k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get client set for deploy stroppy")
	}

	nodesCount := mongo.k.Nodes - 2 //исключаем мастер-ноду и ноду со stroppy
	storagesVolume := 100           //временный хардкод т.к. нет идей, как пробросить сюда размер диска без импортов вне пакета kubernetes
	pvcDataNameTemplate := "data-volume-mongodb-cluster"
	pvcLogNameTemplate := "logs-volume-mongodb-cluster"

	// инициализируем сущности pvc
	var pvcTemplateSpec corev1.PersistentVolumeClaimSpecApplyConfiguration
	var resoursesSpec corev1.ResourceRequirementsApplyConfiguration

	dataResourses := v1.ResourceList{}
	logsResourses := v1.ResourceList{}

	// создаем "шаблон" для дальнейшего использования обоими pvc
	pvcTemplateConfig := corev1.PersistentVolumeClaim("", kubernetes.ResourceDefaultNamespace)

	// задаем общие параметры в спецификации
	pvcTemplateSpec.WithStorageClassName(kubernetes.VolumeStorageClassName)
	pvcTemplateSpec.WithAccessModes(v1.ReadWriteOnce)

	entries := make(map[string]string)
	entries["app"] = "mongodb-cluster-svc"
	pvcTemplateConfig.WithSpec(&pvcTemplateSpec)
	pvcTemplateConfig.WithLabels(entries)

	for i := 0; i < nodesCount; i++ {

		pvcDataName := fmt.Sprintf("%v-%v", pvcDataNameTemplate, i)
		pvcLogName := fmt.Sprintf("%v-%v", pvcLogNameTemplate, i)

		dataPvcConfig := pvcTemplateConfig.WithName(pvcDataName)

		// распределяем общий объем диска: 80% - data, 20% - logs
		storageVolumeString := fmt.Sprintf("%vG", float64(storagesVolume)*0.8)
		diskVolume := resource.MustParse(storageVolumeString)

		dataResourses[v1.ResourceStorage] = diskVolume

		resoursesSpec.WithLimits(dataResourses)
		resoursesSpec.WithRequests(dataResourses)

		dataPvcConfig.Spec.WithResources(&resoursesSpec)

		err = tools.Retry("apply data volume",
			func() (err error) {
				_, err = clientSet.CoreV1().PersistentVolumeClaims(kubernetes.ResourceDefaultNamespace).Apply(context.TODO(), dataPvcConfig, metav1.ApplyOptions{
					TypeMeta:     metav1.TypeMeta{},
					DryRun:       []string{},
					Force:        false,
					FieldManager: "stroppy-deploy",
				})
				return err
			},
			tools.RetryStandardRetryCount,
			tools.RetryStandardWaitingTime)

		if err != nil {
			return merry.Prepend(err, "failed to apply data volume")
		}

		logPvcConfig := pvcTemplateConfig.WithName(pvcLogName)

		storageVolumeString = fmt.Sprintf("%vG", float64(storagesVolume)*0.2)
		diskVolume = resource.MustParse(storageVolumeString)
		logsResourses[v1.ResourceStorage] = diskVolume

		resoursesSpec.WithLimits(logsResourses)
		resoursesSpec.WithRequests(logsResourses)

		logPvcConfig.Spec.WithResources(&resoursesSpec)

		err = tools.Retry("apply log volume",
			func() (err error) {
				_, err = clientSet.CoreV1().PersistentVolumeClaims(kubernetes.ResourceDefaultNamespace).Apply(context.TODO(), logPvcConfig, metav1.ApplyOptions{
					TypeMeta:     metav1.TypeMeta{},
					DryRun:       []string{},
					Force:        false,
					FieldManager: "stroppy-deploy",
				})
				return err
			},
			tools.RetryStandardRetryCount,
			tools.RetryStandardWaitingTime)

		if err != nil {
			return merry.Prepend(err, "failed to apply log volume")
		}

	}

	return nil
}
