// Copyright 2021 Coinbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package polygon

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	mocks "github.com/maticnetwork/polygon-rosetta/mocks/polygon"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sync/semaphore"
)

const (
	methodBatchCallContext = "BatchCallContext"

	methodCallContext = "CallContext"

	rpcGetBlockByHash = "eth_getBlockByHash"

	rpcGetBlockByNumber = "eth_getBlockByNumber"

	rpcGetAuthor = "bor_getAuthor"

	rpcTraceBlockByHash = "debug_traceBlockByHash"

	// Encoded ERC20 decimals (read) method
	decimalsABIEncoded = "0x313ce567"

	// Encoded ERC20 symbol (read) method
	symbolABIEncoded = "0x95d89b41"
)

func testTraceConfig() (*eth.TraceConfig, error) {
	loadedFile, err := ioutil.ReadFile("call_tracer.js")
	if err != nil {
		return nil, fmt.Errorf("%w: could not load tracer file", err)
	}

	loadedTracer := string(loadedFile)
	return &eth.TraceConfig{
		Timeout: &tracerTimeout,
		Tracer:  &loadedTracer,
	}, nil
}

// Wrapper around Client
type mockClient struct {
	ctx     context.Context
	t       *testing.T
	jsonRPC *mocks.JSONRPC
	graphQL *mocks.GraphQL

	client Client
}

func createMockClient(ctx context.Context, t *testing.T) (*mockClient, error) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	tc, err := testTraceConfig()
	if err != nil {
		return nil, err
	}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	if err != nil {
		return nil, err
	}

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		tc:              tc,
		currencyFetcher: cf,
		p:               params.RopstenChainConfig,
		traceSemaphore:  semaphore.NewWeighted(100), //nolint
	}

	return &mockClient{
		ctx:     ctx,
		t:       t,
		jsonRPC: mockJSONRPC,
		graphQL: mockGraphQL,

		client: *c,
	}, nil
}

func (client mockClient) mockGetBlockByNumber(blockNum int, blockNumHexString string) {
	client.jsonRPC.On(
		methodCallContext,
		client.ctx,
		mock.Anything,
		rpcGetBlockByNumber,
		blockNumHexString,
		true,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*json.RawMessage)

			file, err := ioutil.ReadFile(fmt.Sprintf("testdata/block/block_%d.json", blockNum))
			assert.NoError(client.t, err)

			*r = json.RawMessage(file)
		},
	).Once()
}

func (client mockClient) mockGetBlockByNumberLatest(blockNum int) {
	client.jsonRPC.On(
		methodCallContext,
		client.ctx,
		mock.Anything,
		rpcGetBlockByNumber,
		"latest",
		true,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*json.RawMessage)

			file, err := ioutil.ReadFile(fmt.Sprintf("testdata/block/block_%d.json", blockNum))
			assert.NoError(client.t, err)

			*r = json.RawMessage(file)
		},
	).Once()
}

func (client mockClient) mockGetBlockByHash(blockNum int, blockHash string) {
	client.jsonRPC.On(
		methodCallContext,
		client.ctx,
		mock.Anything,
		rpcGetBlockByHash,
		blockHash,
		true,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*json.RawMessage)

			file, err := ioutil.ReadFile(fmt.Sprintf("testdata/block/block_%d.json", blockNum))
			assert.NoError(client.t, err)

			*r = json.RawMessage(file)
		},
	).Once()
}

func (client mockClient) mockGetAuthor(blockNumHexString string, author string) {
	client.jsonRPC.On(
		methodCallContext,
		client.ctx,
		mock.Anything,
		rpcGetAuthor,
		blockNumHexString,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*string)
			*r = author
		},
	).Once()
}

func (client mockClient) mockTraceBlockByHash(blockHash string) {
	client.jsonRPC.On(
		methodCallContext,
		client.ctx,
		mock.Anything,
		rpcTraceBlockByHash,
		common.HexToHash(blockHash),
		client.client.tc,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*json.RawMessage)

			file, err := ioutil.ReadFile(
				fmt.Sprintf("testdata/block/block_trace_%s.json", blockHash))
			assert.NoError(client.t, err)

			*r = json.RawMessage(file)
		},
	).Once()
}

func (client mockClient) mockGetTransactionReceipts(txHashes []string) {
	client.jsonRPC.On(
		methodBatchCallContext,
		client.ctx,
		mock.Anything,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			// args[0] is ctx, args[1] is reqs (each of which is a rpc.BatchElem)
			r := args.Get(1).([]rpc.BatchElem)

			assert.Len(client.t, r, len(txHashes))

			for index, elem := range txHashes {
				// need to do this for multiple tx hashes within the block
				assert.Equal(
					client.t,
					elem,
					r[index].Args[0],
				)

				file, err := ioutil.ReadFile(fmt.Sprintf("testdata/transaction/tx_receipt_%s.json", elem))
				assert.NoError(client.t, err)

				receipt := new(types.Receipt)
				assert.NoError(client.t, receipt.UnmarshalJSON(file))
				*(r[index].Result.(**types.Receipt)) = receipt
			}
		},
	).Once()
}

func (client mockClient) mockTokenDetails(tokenAddresses []string) {
	for _, addr := range tokenAddresses {
		// Mock symbol calls
		result, err := ioutil.ReadFile(fmt.Sprintf("testdata/token_contracts/symbol_%s.json", addr))
		assert.NoError(client.t, err)
		client.graphQL.On(
			"Query",
			client.ctx,
			buildGraphqlCallQuery("", addr, symbolABIEncoded),
		).Return(
			string(result),
			nil,
		).Once()
		// Mock decimals calls
		result, err = ioutil.ReadFile(fmt.Sprintf("testdata/token_contracts/decimals_%s.json", addr))
		assert.NoError(client.t, err)
		client.graphQL.On(
			"Query",
			client.ctx,
			buildGraphqlCallQuery("", addr, decimalsABIEncoded),
		).Return(
			string(result),
			nil,
		).Once()
	}
}

func (client mockClient) mockGraphqlCall(query string, result []byte) {
	client.graphQL.On(
		"Query",
		client.ctx,
		query,
	).Return(
		string(result),
		nil,
	).Once()
}

func (client mockClient) mockGraphqlBalance(query string, result []byte) {
	client.graphQL.On(
		"Query",
		client.ctx,
		query,
	).Return(
		string(result),
		nil,
	).Once()
}
