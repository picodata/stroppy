package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func GetAddressMap(wd, provider string) (mapIP *MapAddresses, err error) {
	stateFilePath := filepath.Join(wd, stateFileName)

	mapIP, err = getAddressMap(stateFilePath, provider)
	return
}

func getAddressMap(stateFilePath, provider string) (mapIP *MapAddresses, err error) {
	/* Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
	 * своя пара - внешний (NAT) и внутренний ip.
	 * Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
	 * т.к. использование encoding/json
	 * влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
	 * по переданному тегу json, который можно преобразовать в том числе в строку. */

	var data []byte
	if data, err = ioutil.ReadFile(stateFilePath); err != nil {
		err = merry.Prepend(err, "failed to read file terraform.tfstate")
		return
	}

	mapIP = &MapAddresses{}
	switch provider {
	case "yandex":
		{
			masterExternalIPArray := gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0")

			mapIP.MasterExternalIP = masterExternalIPArray.
				Get("attributes").
				Get("network_interface.0").
				Get("nat_ip_address").Str

			masterInternalIPArray := gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0")

			mapIP.MasterInternalIP = masterInternalIPArray.
				Get("attributes").
				Get("network_interface.0").
				Get("ip_address").Str

			metricsExternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.MetricsExternalIP = metricsExternalIPArray.
				Get("instances.0").
				Get("network_interface.0").
				Get("nat_ip_address").Str

			metricsInternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.MetricsInternalIP = metricsInternalIPArray.
				Get("instances.0").
				Get("network_interface.0").
				Get("ip_address").Str

			ingressExternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.IngressExternalIP = ingressExternalIPArray.
				Get("instances.1").
				Get("network_interface.0").
				Get("nat_ip_address").Str

			ingressInternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.IngressInternalIP = ingressInternalIPArray.
				Get("instances.1").
				Get("network_interface.0").
				Get("ip_address").Str

			postgresExternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.DatabaseExternalIP = postgresExternalIPArray.
				Get("instances.2").
				Get("network_interface.0").
				Get("nat_ip_address").Str

			postgresInternalIPArray := gjson.Parse(string(data)).
				Get("resources.2").
				Get("instances.0").
				Get("attributes")

			mapIP.PostgresInternalIP = postgresInternalIPArray.
				Get("instances.2").
				Get("network_interface.0").
				Get("ip_address").Str
		}

	case "oracle":
		{
			mapIP.MasterInternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.0").Str

			mapIP.MetricsInternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.1").Str

			mapIP.IngressInternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.2").Str

			mapIP.PostgresInternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.3").Str

			mapIP.MasterExternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.0").Str

			mapIP.MetricsExternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.1").Str

			mapIP.IngressExternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.2").Str

			mapIP.DatabaseExternalIP = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.3").Str
		}
	}

	return
}

func GetIQNStorage(wd string, workersCount int) (iqnMap map[string]string, err error) {

	stateFilePath := filepath.Join(wd, stateFileName)
	var data []byte

	if data, err = ioutil.ReadFile(stateFilePath); err != nil {
		err = merry.Prepend(err, "failed to read file terraform.tfstate")
		return
	}

	iqnMap = make(map[string]string)
	masterInstance := "instances.0"
	iqnMap["master"] = gjson.Parse(string(data)).Get("resources.9").Get(masterInstance).Get("attributes").Get("iqn").Str

	for i := 1; i <= workersCount; i++ {
		workerInstance := fmt.Sprintf("instances.%v", i)
		worker := fmt.Sprintf("worker-%v", i)
		iqnMap[worker] = gjson.Parse(string(data)).Get("resources.9").Get(workerInstance).Get("attributes").Get("iqn").Str
	}

	return iqnMap, nil

}

func AddNetworkStorage(wd string, nodes int, addressMap MapAddresses, provider string) error {

	iqnMap, err := GetIQNStorage(wd, nodes)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	//временное решение до перехода на поддержку динамического кол-ва нод
	var addressArray []string
	addressMapData := reflect.ValueOf(addressMap)
	for i := 0; i < addressMapData.NumField(); i++ {
		addressArray = append(addressArray, addressMapData.Field(i).Interface().(string))
	}

	// добавить добавление storage на мастер
	for i := range addressArray {
		client, err := engineSsh.CreateClient(wd, addressArray[i], provider, false)
		if err != nil {
			return merry.Prepend(err, "failed to create ssh client")
		}

		session, err := client.GetNewSession()
		if err != nil {
			return merry.Prepend(err, "failed to get ssh session")
		}

		worker := fmt.Sprintf("worker-%v", i+1)

		addStorageCmd := fmt.Sprintf(addStorageCmdTemplate, iqnMap[worker], iqnMap[worker], iqnMap[worker])

		cmdResult, err := session.CombinedOutput(addStorageCmd)
		if err != nil {
			llog.Errorf(string(cmdResult))
			return merry.Prepend(err, "failed to get ssh session")
		}

	}

	return nil
}
