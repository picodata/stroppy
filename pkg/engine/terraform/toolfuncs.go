package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
)

func GetAddressMap(wd, provider string, nodes int) (mapIP *MapAddresses, err error) {
	stateFilePath := filepath.Join(wd, stateFileName)

	mapIP, err = getAddressMap(stateFilePath, provider, nodes)
	return
}

func getAddressMap(stateFilePath, provider string, nodes int) (mapIPAddresses map[string]string, err error) {
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
	//mapIP = &MapAddresses{}

	mapIPAddresses = make(map[string]string)
	masterKeyInternal := "master_internal"
	masterKeyExternal := "master_external"
	workerKeyExternal := "worker_external_%v"
	workerKeyInternal := "worker_internal_%v"
	oracleInstanceValue := "value.0.%v"
	yandexInstanceValue := "instances.%v"

	switch provider {
	case "yandex":
		{
			mapIPAddresses[masterKeyExternal] = gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0").
				Get("attributes").
				Get("network_interface.0").
				Get("nat_ip_address").Str
			mapIPAddresses[masterKeyInternal] = gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0").
				Get("attributes").
				Get("network_interface.0").
				Get("ip_address").Str

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKeyExternal, i)
				currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
				mapIPAddresses[key] = gjson.Parse(string(data)).
					Get("resources.2").
					Get("instances.0").
					Get("attributes").
					Get(currentInstanceValue).
					Get("network_interface.0").
					Get("nat_ip_address").Str
			}

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKeyInternal, i)
				currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
				mapIPAddresses[key] = gjson.Parse(string(data)).
					Get("resources.2").
					Get("instances.0").
					Get("attributes").
					Get(currentInstanceValue).
					Get("network_interface.0").
					Get("ip_address").Str
			}

		}

	case "oracle":
		{
			mapIPAddresses[masterKeyExternal] = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.0").Str

			mapIPAddresses[masterKeyInternal] = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.0").Str

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKeyInternal, i)
				currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
				mapIPAddresses[key] = gjson.Parse(string(data)).
					Get("outputs").
					Get("instance_private_ips").
					Get(currentInstanceValue).Str
			}

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKeyExternal, i)
				currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
				mapIPAddresses[key] = gjson.Parse(string(data)).
					Get("outputs").
					Get("instance_public_ips").
					Get(currentInstanceValue).Str
			}
		}
	}

	return
}
