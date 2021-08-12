package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
)

func GetAddressMap(wd, provider string, nodes int) (mapIP map[string]map[string]string, err error) {
	stateFilePath := filepath.Join(wd, stateFileName)

	mapIP, err = getAddressMap(stateFilePath, provider, nodes)

	return
}

// getAddressMap - получить карту(map) ip-адресов для работы с кластером
func getAddressMap(stateFilePath, provider string, nodes int) (mapIPAddresses map[string]map[string]string, err error) {
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

	mapIPAddresses = make(map[string]map[string]string)
	workerKey := "worker-%v"
	oracleInstanceValue := "value.0.%v"
	yandexInstanceValue := "instances.%v"
	externalAddress := make(map[string]string)
	internalAddress := make(map[string]string)

	switch provider {
	case "yandex":
		{

			externalAddress["master"] = gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0").
				Get("attributes").
				Get("network_interface.0").
				Get("nat_ip_address").Str

			internalAddress["master"] = gjson.Parse(string(data)).
				Get("resources.1").
				Get("instances.0").
				Get("attributes").
				Get("network_interface.0").
				Get("ip_address").Str

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKey, i)
				currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
				externalAddress[key] = gjson.Parse(string(data)).
					Get("resources.2").
					Get("instances.0").
					Get("attributes").
					Get(currentInstanceValue).
					Get("network_interface.0").
					Get("nat_ip_address").Str
			}

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKey, i)
				currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
				internalAddress[key] = gjson.Parse(string(data)).
					Get("resources.2").
					Get("instances.0").
					Get("attributes").
					Get(currentInstanceValue).
					Get("network_interface.0").
					Get("ip_address").Str
			}

			mapIPAddresses["external"] = externalAddress
			mapIPAddresses["internal"] = internalAddress

		}

	case "oracle":
		{
			externalAddress["master"] = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_public_ips").
				Get("value.0.0").Str

			internalAddress["master"] = gjson.Parse(string(data)).
				Get("outputs").
				Get("instance_private_ips").
				Get("value.0.0").Str

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKey, i)
				currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
				externalAddress[key] = gjson.Parse(string(data)).
					Get("outputs").
					Get("instance_public_ips").
					Get(currentInstanceValue).Str
			}

			for i := 1; i <= nodes-1; i++ {
				key := fmt.Sprintf(workerKey, i)
				currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
				internalAddress[key] = gjson.Parse(string(data)).
					Get("outputs").
					Get("instance_private_ips").
					Get(currentInstanceValue).Str
			}

			mapIPAddresses["external"] = externalAddress
			mapIPAddresses["internal"] = internalAddress
		}
	}

	return
}
