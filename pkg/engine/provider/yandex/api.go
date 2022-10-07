package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"atomicgo.dev/cursor"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	yandexCloudIAMDomain        = "iam.api.cloud.yandex.net"
	yandexCloudIAMPath          = "/iam/v1/tokens"
	yandexCloudAPIDomain        = "compute.api.cloud.yandex.net"
	yandexCloudAPIInstancesPath = "/compute/v1/instances"
)

var (
	errOauthTokenVarUndefined = errors.New("variable TF_VAR_token for yandex cloud undefined")
	errEmptyStatus            = errors.New("instance status received from yandex cloud in empty")
)

type Header struct {
	name  string
	value string
}

type IAMResponse struct {
	IamToken  string `json:"iamToken"`
	ExpiresAt string `json:"expiresAt"`
}

type NodeStatus struct {
	Status string `json:"status"`
}

type StatusPrinter struct {
	NodesRows    *[][]string
	StatusStings *strings.Builder
	StatusTable  *tablewriter.Table
	PrintArea    *cursor.Area
}

type NodesRows [][]string

func (nodeRows NodesRows) Len() int {
	return len(nodeRows)
}

func (nodeRows NodesRows) Less(i, j int) bool {
	return nodeRows[i][0] < nodeRows[j][0]
}

func (nodeRows NodesRows) Swap(i, j int) {
	nodeRows[i], nodeRows[j] = nodeRows[j], nodeRows[i]
}

func createStatusPrinter(
	provider *Provider,
	statusStrings *strings.Builder,
	statusTable *tablewriter.Table,
	printArea *cursor.Area,
) StatusPrinter {
	ipAddresses := provider.GetInstancesAddresses().GetWorkersAndMastersAddrPairs()
	index := 0
	nodesRaws := make([][]string, len(ipAddresses))

	for name, node := range provider.GetNodesInfo() {
		// Name | ID | Status | Internal | External
		nodesRaws[index] = []string{
			name,
			node.InstanceID,
			"",
			ipAddresses[name].Internal,
			ipAddresses[name].External,
		}

		index++
	}

	sort.Sort(NodesRows(nodesRaws))

	return StatusPrinter{
		NodesRows:    &nodesRaws,
		StatusStings: statusStrings,
		StatusTable:  statusTable,
		PrintArea:    printArea,
	}
}

func (statusPrinter *StatusPrinter) updateStatus(index int, nodeStatus *NodeStatus) error {
	if nodeStatus == nil {
		return errEmptyStatus
	}

	(*statusPrinter.NodesRows)[index][2] = nodeStatus.Status

	statusPrinter.StatusStings.Reset()
	statusPrinter.StatusTable.ClearRows()
	statusPrinter.StatusTable.AppendBulk(*statusPrinter.NodesRows)
	statusPrinter.StatusTable.Render()
	statusPrinter.PrintArea.Update(statusPrinter.StatusStings.String())

	return nil
}

func getIamToken() (*IAMResponse, error) {
	var (
		res  *http.Response
		body []byte
		err  error
	)

	oauthToken, ok := os.LookupEnv("TF_VAR_token")
	if !ok {
		return nil, errOauthTokenVarUndefined
	}

	iamURL := url.URL{ //nolint
		Scheme: "https",
		Host:   yandexCloudIAMDomain,
		Path:   yandexCloudIAMPath,
	}

	iamReq := map[string]string{
		"yandexPassportOauthToken": oauthToken,
	}

	if body, err = json.Marshal(iamReq); err != nil {
		return nil, errors.Wrap(err, "failed to serialize req body into json")
	}

	if res, err = reqResource(
		http.MethodPost, &iamURL, bytes.NewReader(body), []Header{},
	); err != nil {
		return nil, errors.Wrap(err, "failed to get IAM token")
	}

	if res.Body != nil {
		defer func() { res.Body.Close() }()
	}

	var iamToken IAMResponse

	if err = json.NewDecoder(res.Body).Decode(&iamToken); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize response IAMResponse")
	}

	return &iamToken, nil
}

func reqResource(
	method string,
	instanceURL *url.URL,
	body io.Reader,
	headers []Header,
) (*http.Response, error) {
	var (
		req *http.Request
		res *http.Response
		err error
	)

	reqContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second)

	defer ctxCloseFn()

	if req, err = http.NewRequestWithContext(
		reqContext,
		method,
		instanceURL.String(),
		body,
	); err != nil {
		return nil, errors.Wrap(err, "failed to cunstruct request")
	}

	for _, header := range headers {
		req.Header.Add(header.name, header.value)
	}

	if res, err = http.DefaultClient.Do(req); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to do reqest to %s", instanceURL))
	}

	return res, nil
}

func retrieveStatus(
	index int,
	iamToken *IAMResponse,
	statusPrinter *StatusPrinter,
	mutex *sync.RWMutex,
	nodeWg *sync.WaitGroup,
	errs chan error,
) {
	var (
		nodeResp   *http.Response
		nodeStatus NodeStatus
		err        error
	)

	instanceURL := url.URL{ //nolint
		Scheme: "https",
		Host:   yandexCloudAPIDomain,
		Path: path.Join(
			yandexCloudAPIInstancesPath,
			(*statusPrinter.NodesRows)[index][1],
		),
	}

	defer func() {
		mutex.Unlock()
		nodeWg.Done()

		if nodeResp.Body != nil {
			nodeResp.Body.Close()
		}
	}()

	for {
		time.Sleep(time.Second)

		if nodeResp, err = reqResource(
			http.MethodGet, &instanceURL,
			nil,
			[]Header{
				{"Authorization", "Bearer " + iamToken.IamToken},
				{"Accept", "application/json"},
			},
		); err != nil {
			errs <- errors.Wrap(err, "failed to get request to instance (node) resource")

			logrus.Tracef("request to node %d ended with error", index)

			break
		}

		if !mutex.TryLock() {
			continue
		}

		if err = json.NewDecoder(nodeResp.Body).Decode(&nodeStatus); err != nil {
			errs <- errors.Wrap(err, "failed to deserialize node status resource")

			logrus.Tracef("error whiled deserialization node %d status response", index)

			break
		}

		if err = statusPrinter.updateStatus(index, &nodeStatus); err != nil {
			errs <- errors.Wrap(err, "failed to update node status")

			break
		}

		if nodeStatus.Status == "RUNNING" {
			break
		}
	}
}
