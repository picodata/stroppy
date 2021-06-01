package engine

import (
	"bufio"
	"net"
	"strconv"

	llog "github.com/sirupsen/logrus"
)

// IsLocalPortOpen
// проверить доступность порта на localhost
func IsLocalPortOpen(port int) bool {
	address := "localhost:" + strconv.Itoa(port)
	conn, err := net.Listen("tcp", address)
	if err != nil {
		llog.Errorf("port %v at localhost is not available \n", port)
		return false
	}

	if err = conn.Close(); err != nil {
		llog.Errorf("failed to close port %v at localhost after check\n", port)
		return false
	}
	return true
}

// IsRemotePortOpen - проверить доступность порта на удаленной машине кластера
func IsRemotePortOpen(hostname string, port int) bool {
	address := hostname + ":" + strconv.Itoa(port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		llog.Errorf("port %d at '%s' is not available: %v \n", port, address, err)
		return false
	}

	_ = conn.Close()
	return true
}

// HandleReader
// вывести буфер стандартного вывода в отдельном потоке
func HandleReader(reader *bufio.Reader) {
	printOutput := llog.GetLevel() == llog.InfoLevel
	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if printOutput {
			llog.Printf(str)
		}
	}
}
