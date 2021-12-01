// Copyright 2020 Coinbase, Inc.
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
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"testing"

	mocks "github.com/maticnetwork/polygon-rosetta/mocks/polygon"

	RosettaTypes "github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sync/semaphore"
)

const (
	nilAddress = "0x0000000000000000000000000000000000000000"
)

func jsonifyBlock(b *RosettaTypes.Block) (*RosettaTypes.Block, error) {
	bytes, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}

	var bo RosettaTypes.Block
	if err := json.Unmarshal(bytes, &bo); err != nil {
		return nil, err
	}

	return &bo, nil
}

func TestStatus_NotReady(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getBlockByNumber",
		"latest",
		false,
	).Return(
		nil,
	).Once()

	block, timestamp, syncStatus, peers, err := c.Status(ctx)
	assert.Nil(t, block)
	assert.Equal(t, int64(-1), timestamp)
	assert.Nil(t, syncStatus)
	assert.Nil(t, peers)
	assert.True(t, errors.Is(err, ethereum.NotFound))

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestStatus_NotSyncing(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getBlockByNumber",
		"latest",
		false,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			header := args.Get(1).(**types.Header)
			file, err := ioutil.ReadFile("testdata/basic_header.json")
			assert.NoError(t, err)

			*header = new(types.Header)

			assert.NoError(t, (*header).UnmarshalJSON(file))
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_syncing",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			status := args.Get(1).(*json.RawMessage)

			*status = json.RawMessage("false")
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"admin_peers",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			info := args.Get(1).(*[]*p2p.PeerInfo)

			file, err := ioutil.ReadFile("testdata/peers.json")
			assert.NoError(t, err)

			assert.NoError(t, json.Unmarshal(file, info))
		},
	).Once()

	block, timestamp, syncStatus, peers, err := c.Status(ctx)
	assert.Equal(t, &RosettaTypes.BlockIdentifier{
		Hash:  "0x48269a339ce1489cff6bab70eff432289c4f490b81dbd00ff1f81c68de06b842",
		Index: 8916656,
	}, block)
	assert.Equal(t, int64(1603225195000), timestamp)
	assert.Nil(t, syncStatus)
	assert.Equal(t, []*RosettaTypes.Peer{
		{
			PeerID: "16dedaa93519f9ba41a50d77876aae4bfcddfa7cecf232b9abe3ab5bf0b871f3",
			Metadata: map[string]interface{}{
				"caps": []string{
					"eth/63",
					"eth/64",
					"eth/65",
				},
				"enode": "enode://5654cc39fd278c994c451434dfa7b1a44977c52018a87e911368b54daf795955d5a2dc2ece98be5a7e8d0eb245c8ef573c92e04e8b15363f9c713a8127fe7c7b@35.183.116.112:57510", // nolint
				"enr":   "",
				"name":  "Geth/v1.9.22-stable-c71a7e26/linux-amd64/go1.15",
				"protocols": map[string]interface{}{
					"eth": map[string]interface{}{
						"difficulty": float64(31779242235308530),
						"head":       "0x4a01c35e3e2627bf5a735bc9c7f336cb1e6450f93955473008ff64cf01feeef8",
						"version":    float64(65),
					},
				},
			},
		},
		{
			PeerID: "1b75a634fbc9198d73413a0ced02837707d1fd09e4e90b8b90a0abac57113299",
			Metadata: map[string]interface{}{
				"caps": []string{
					"eth/63",
					"eth/64",
					"eth/65",
				},
				"enode": "enode://bead1278155bfabdd51f04a6e896356da2f5687aa1f550bebc540828579522b87e22c67edf90efa651582e40c8c8037eb0f998208cab4a69b52c5e3387671b59@174.129.122.13:30303",                                                    // nolint
				"enr":   "enr:-Je4QICGSLfIHa7vX3bdWnKqWIS7YwmLUP6JVqU5nBhxPpH_X_Uz1pZwVS8a48uESHay1nvz9FtxLYFftpMr3wvFZJ4Qg2V0aMfGhGcn75CAgmlkgnY0gmlwhK6Beg2Jc2VjcDI1NmsxoQO-rRJ4FVv6vdUfBKboljVtovVoeqH1UL68VAgoV5UiuIN0Y3CCdl-DdWRwgnZf", // nolint
				"name":  "Geth/v1.9.15-omnibus-75eb5240/linux-amd64/go1.14.4",
				"protocols": map[string]interface{}{
					"eth": map[string]interface{}{
						"difficulty": float64(31779248439556308),
						"head":       "0x562415e43630bb6d79176ea2fa35ff2a54cee276b678b755831886b1029911bd",
						"version":    float64(65),
					},
				},
			},
		},
	}, peers)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestStatus_NotSyncing_SkipAdminCalls(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
		skipAdminCalls:  true,
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getBlockByNumber",
		"latest",
		false,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			header := args.Get(1).(**types.Header)
			file, err := ioutil.ReadFile("testdata/basic_header.json")
			assert.NoError(t, err)

			*header = new(types.Header)

			assert.NoError(t, (*header).UnmarshalJSON(file))
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_syncing",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			status := args.Get(1).(*json.RawMessage)

			*status = json.RawMessage("false")
		},
	).Once()

	adminPeersSkipped := true
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"admin_peers",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			adminPeersSkipped = false
		},
	).Maybe()

	block, timestamp, syncStatus, peers, err := c.Status(ctx)
	assert.True(t, adminPeersSkipped)
	assert.Equal(t, &RosettaTypes.BlockIdentifier{
		Hash:  "0x48269a339ce1489cff6bab70eff432289c4f490b81dbd00ff1f81c68de06b842",
		Index: 8916656,
	}, block)
	assert.Equal(t, int64(1603225195000), timestamp)
	assert.Nil(t, syncStatus)
	assert.Equal(t, []*RosettaTypes.Peer{}, peers)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestStatus_Syncing(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getBlockByNumber",
		"latest",
		false,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			header := args.Get(1).(**types.Header)
			file, err := ioutil.ReadFile("testdata/basic_header.json")
			assert.NoError(t, err)

			*header = new(types.Header)

			assert.NoError(t, (*header).UnmarshalJSON(file))
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_syncing",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			progress := args.Get(1).(*json.RawMessage)
			file, err := ioutil.ReadFile("testdata/syncing_info.json")
			assert.NoError(t, err)

			*progress = json.RawMessage(file)
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"admin_peers",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			info := args.Get(1).(*[]*p2p.PeerInfo)

			file, err := ioutil.ReadFile("testdata/peers.json")
			assert.NoError(t, err)

			assert.NoError(t, json.Unmarshal(file, info))
		},
	).Once()

	block, timestamp, syncStatus, peers, err := c.Status(ctx)
	assert.Equal(t, &RosettaTypes.BlockIdentifier{
		Hash:  "0x48269a339ce1489cff6bab70eff432289c4f490b81dbd00ff1f81c68de06b842",
		Index: 8916656,
	}, block)
	assert.Equal(t, int64(1603225195000), timestamp)
	assert.Equal(t, &RosettaTypes.SyncStatus{
		CurrentIndex: RosettaTypes.Int64(25),
		TargetIndex:  RosettaTypes.Int64(8916760),
	}, syncStatus)
	assert.Equal(t, []*RosettaTypes.Peer{
		{
			PeerID: "16dedaa93519f9ba41a50d77876aae4bfcddfa7cecf232b9abe3ab5bf0b871f3",
			Metadata: map[string]interface{}{
				"caps": []string{
					"eth/63",
					"eth/64",
					"eth/65",
				},
				"enode": "enode://5654cc39fd278c994c451434dfa7b1a44977c52018a87e911368b54daf795955d5a2dc2ece98be5a7e8d0eb245c8ef573c92e04e8b15363f9c713a8127fe7c7b@35.183.116.112:57510", // nolint
				"enr":   "",
				"name":  "Geth/v1.9.22-stable-c71a7e26/linux-amd64/go1.15",
				"protocols": map[string]interface{}{
					"eth": map[string]interface{}{
						"difficulty": float64(31779242235308530),
						"head":       "0x4a01c35e3e2627bf5a735bc9c7f336cb1e6450f93955473008ff64cf01feeef8",
						"version":    float64(65),
					},
				},
			},
		},
		{
			PeerID: "1b75a634fbc9198d73413a0ced02837707d1fd09e4e90b8b90a0abac57113299",
			Metadata: map[string]interface{}{
				"caps": []string{
					"eth/63",
					"eth/64",
					"eth/65",
				},
				"enode": "enode://bead1278155bfabdd51f04a6e896356da2f5687aa1f550bebc540828579522b87e22c67edf90efa651582e40c8c8037eb0f998208cab4a69b52c5e3387671b59@174.129.122.13:30303",                                                    // nolint
				"enr":   "enr:-Je4QICGSLfIHa7vX3bdWnKqWIS7YwmLUP6JVqU5nBhxPpH_X_Uz1pZwVS8a48uESHay1nvz9FtxLYFftpMr3wvFZJ4Qg2V0aMfGhGcn75CAgmlkgnY0gmlwhK6Beg2Jc2VjcDI1NmsxoQO-rRJ4FVv6vdUfBKboljVtovVoeqH1UL68VAgoV5UiuIN0Y3CCdl-DdWRwgnZf", // nolint
				"name":  "Geth/v1.9.15-omnibus-75eb5240/linux-amd64/go1.14.4",
				"protocols": map[string]interface{}{
					"eth": map[string]interface{}{
						"difficulty": float64(31779248439556308),
						"head":       "0x562415e43630bb6d79176ea2fa35ff2a54cee276b678b755831886b1029911bd",
						"version":    float64(65),
					},
				},
			},
		},
	}, peers)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestStatus_Syncing_SkipAdminCalls(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
		skipAdminCalls:  true,
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getBlockByNumber",
		"latest",
		false,
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			header := args.Get(1).(**types.Header)
			file, err := ioutil.ReadFile("testdata/basic_header.json")
			assert.NoError(t, err)

			*header = new(types.Header)

			assert.NoError(t, (*header).UnmarshalJSON(file))
		},
	).Once()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_syncing",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			progress := args.Get(1).(*json.RawMessage)
			file, err := ioutil.ReadFile("testdata/syncing_info.json")
			assert.NoError(t, err)

			*progress = json.RawMessage(file)
		},
	).Once()

	adminPeersSkipped := true
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"admin_peers",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			adminPeersSkipped = false
		},
	).Maybe()

	block, timestamp, syncStatus, peers, err := c.Status(ctx)
	assert.True(t, adminPeersSkipped)
	assert.Equal(t, &RosettaTypes.BlockIdentifier{
		Hash:  "0x48269a339ce1489cff6bab70eff432289c4f490b81dbd00ff1f81c68de06b842",
		Index: 8916656,
	}, block)
	assert.Equal(t, int64(1603225195000), timestamp)
	assert.Equal(t, &RosettaTypes.SyncStatus{
		CurrentIndex: RosettaTypes.Int64(25),
		TargetIndex:  RosettaTypes.Int64(8916760),
	}, syncStatus)
	assert.Equal(t, []*RosettaTypes.Peer{}, peers)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

var mockCurrency = &RosettaTypes.Currency{
	Symbol:   "mock",
	Decimals: int32(18),
	Metadata: map[string]interface{}{
		"token_address": "0x4a1b31b1bd1c691622bb27a55155c01137bd532c",
	},
}

var mockInvalidCurrency = &RosettaTypes.Currency{
	Symbol:   "invalid",
	Decimals: int32(18),
	Metadata: map[string]interface{}{
		"token_address": "0xdeadbeef",
	},
}

func TestBalance(t *testing.T) {
	var tests = map[string]struct {
		address                string
		blockHash              string
		blockQuery             string
		accountBalanceFile     string
		accountIdentifier      *RosettaTypes.AccountIdentifier
		partialBlockIdentifier *RosettaTypes.PartialBlockIdentifier
		currencies             []*RosettaTypes.Currency
		expectedResponse       *RosettaTypes.AccountBalanceResponse
		error                  error
	}{
		"latest": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			blockHash:          "",
			blockQuery:         "",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: nil,
			currencies:             nil,
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"historical hash": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         `hash: "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb"`,
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Hash:  RosettaTypes.String("0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb"),
				Index: RosettaTypes.Int64(18678892),
			},
			currencies: nil,
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"historical index": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         "number: 8165",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(8165),
			},
			currencies: nil,
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"invalid address": {
			address:            "0x4cfc400fed52f9681b42454c2db4b18ab98f8de",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         "",
			accountBalanceFile: "testdata/account_balance/invalid_address.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x4cfc400fed52f9681b42454c2db4b18ab98f8de",
			},
			partialBlockIdentifier: nil,
			currencies:             nil,
			expectedResponse:       nil,
			error:                  fmt.Errorf("[{\"message\":\"hex string of odd length\",\"path\":null}]"),
		},
		"invalid hash": {
			address:            "0x2f93B2f047E05cdf602820Ac4B3178efc2b43D55",
			blockHash:          "0x7d2a2713026a0e66f131878de2bb2df2fff6c24562c1df61ec0265e5fedf2626",
			blockQuery:         `hash: "0x7d2a2713026a0e66f131878de2bb2df2fff6c24562c1df61ec0265e5fedf2626"`,
			accountBalanceFile: "testdata/account_balance/invalid_block.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x2f93B2f047E05cdf602820Ac4B3178efc2b43D55",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Hash: RosettaTypes.String(
					"0x7d2a2713026a0e66f131878de2bb2df2fff6c24562c1df61ec0265e5fedf2626",
				),
			},
			currencies:       nil,
			expectedResponse: nil,
			error:            fmt.Errorf("[{\"message\":\"header for hash not found\",\"path\":[\"block\"]}]"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mockClient, err := createMockClient(ctx, t)
			assert.NoError(t, err)

			result, err := ioutil.ReadFile(test.accountBalanceFile)
			assert.NoError(t, err)

			balanceQuery := buildGraphqlBalanceQuery(test.blockQuery, test.address)
			mockClient.mockGraphqlBalance(balanceQuery, result)

			resp, err := mockClient.client.Balance(
				ctx,
				test.accountIdentifier,
				test.partialBlockIdentifier,
				test.currencies,
			)

			assert.Equal(t, test.expectedResponse, resp)
			assert.Equal(t, test.error, err)

			mockClient.jsonRPC.AssertExpectations(t)
			mockClient.graphQL.AssertExpectations(t)
		})
	}
}

func TestBalance_ERC20Token(t *testing.T) {
	var tests = map[string]struct {
		address                string
		accountBalanceFile     string
		blockHash              string
		blockQuery             string
		contractAddress        string
		dataPayload            string
		validBalanceFile       string
		accountIdentifier      *RosettaTypes.AccountIdentifier
		partialBlockIdentifier *RosettaTypes.PartialBlockIdentifier
		currencies             []*RosettaTypes.Currency
		expectedResponse       *RosettaTypes.AccountBalanceResponse
		error                  error
	}{
		"latest": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         "",
			contractAddress:    "0x4a1b31b1bd1c691622bb27a55155c01137bd532c",
			dataPayload:        "0x70a0823100000000000000000000000044856eeaa735ba08be507d2d98411271ceaa6baa",
			validBalanceFile:   "testdata/account_balance_with_tokens/valid_balance.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: nil,
			currencies:             []*RosettaTypes.Currency{Currency, mockCurrency},
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
					{
						Value:    "10000000000000000000000",
						Currency: mockCurrency,
						Metadata: map[string]interface{}{
							"status":  int64(1),
							"gasUsed": int64(36875),
						},
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"historical hash": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         `hash: "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb"`,
			contractAddress:    "0x4a1b31b1bd1c691622bb27a55155c01137bd532c",
			dataPayload:        "0x70a0823100000000000000000000000044856eeaa735ba08be507d2d98411271ceaa6baa",
			validBalanceFile:   "testdata/account_balance_with_tokens/valid_balance.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Hash: RosettaTypes.String("0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb"),
			},
			currencies: []*RosettaTypes.Currency{Currency, mockCurrency},
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
					{
						Value:    "10000000000000000000000",
						Currency: mockCurrency,
						Metadata: map[string]interface{}{
							"status":  int64(1),
							"gasUsed": int64(36875),
						},
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"historical index": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         "number: 18678892",
			contractAddress:    "0x4a1b31b1bd1c691622bb27a55155c01137bd532c",
			dataPayload:        "0x70a0823100000000000000000000000044856eeaa735ba08be507d2d98411271ceaa6baa",
			validBalanceFile:   "testdata/account_balance_with_tokens/valid_balance.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(18678892),
			},
			currencies: []*RosettaTypes.Currency{Currency, mockCurrency},
			expectedResponse: &RosettaTypes.AccountBalanceResponse{
				BlockIdentifier: &RosettaTypes.BlockIdentifier{
					Hash:  "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
					Index: 18678892,
				},
				Balances: []*RosettaTypes.Amount{
					{
						Value:    "10372550232136640000000",
						Currency: Currency,
					},
					{
						Value:    "10000000000000000000000",
						Currency: mockCurrency,
						Metadata: map[string]interface{}{
							"status":  int64(1),
							"gasUsed": int64(36875),
						},
					},
				},
				Metadata: map[string]interface{}{
					"code":  "0x",
					"nonce": int64(0),
				},
			},
			error: nil,
		},
		"invalid contract address": {
			address:            "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			accountBalanceFile: "testdata/account_balance/0x44856eeaa735ba08be507d2d98411271ceaa6baa.json",
			blockHash:          "0x10ac7a3e929ef050360228b4812755d7aab9448a5dd2f7898c4593048929a2bb",
			blockQuery:         "",
			contractAddress:    fmt.Sprintf("%s", mockInvalidCurrency.Metadata[ContractAddressKey]),
			dataPayload:        "0x70a0823100000000000000000000000044856eeaa735ba08be507d2d98411271ceaa6baa",
			validBalanceFile:   "testdata/account_balance_with_tokens/valid_balance.json",
			accountIdentifier: &RosettaTypes.AccountIdentifier{
				Address: "0x44856eeaa735ba08be507d2d98411271ceaa6baa",
			},
			partialBlockIdentifier: nil,
			currencies:             []*RosettaTypes.Currency{Currency, mockInvalidCurrency},
			error:                  fmt.Errorf("invalid contract address 0xdeadbeef"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mockClient, err := createMockClient(ctx, t)
			assert.NoError(t, err)

			result, err := ioutil.ReadFile(test.accountBalanceFile)
			assert.NoError(t, err)

			balanceQuery := buildGraphqlBalanceQuery(test.blockQuery, test.address)
			mockClient.mockGraphqlBalance(balanceQuery, result)

			// Only mock graphql calls if contract address is valid; otherwise, we wouldn't make those calls
			// in the first place
			_, ok := ChecksumAddress(test.contractAddress)
			if ok {
				result, err := ioutil.ReadFile(test.validBalanceFile)
				assert.NoError(t, err)

				callQuery := buildGraphqlCallQuery(test.blockQuery, test.contractAddress, test.dataPayload)
				mockClient.mockGraphqlCall(callQuery, result)
			}

			resp, err := mockClient.client.Balance(
				ctx,
				test.accountIdentifier,
				test.partialBlockIdentifier,
				test.currencies,
			)

			assert.Equal(t, test.expectedResponse, resp)
			assert.Equal(t, test.error, err)

			mockClient.jsonRPC.AssertExpectations(t)
			mockClient.graphQL.AssertExpectations(t)
		})
	}
}

func TestCall(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()

	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getTransactionReceipt",
		common.HexToHash("0xb358c6958b1cab722752939cbb92e3fec6b6023de360305910ce80c56c3dad9d"),
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(**types.Receipt)

			file, err := ioutil.ReadFile(
				"testdata/call_0xb358c6958b1cab722752939cbb92e3fec6b6023de360305910ce80c56c3dad9d.json",
			)
			assert.NoError(t, err)

			*r = new(types.Receipt)

			assert.NoError(t, (*r).UnmarshalJSON(file))
		},
	).Once()
	resp, err := c.Call(
		ctx,
		&RosettaTypes.CallRequest{
			Method: "eth_getTransactionReceipt",
			Parameters: map[string]interface{}{
				"tx_hash": "0xb358c6958b1cab722752939cbb92e3fec6b6023de360305910ce80c56c3dad9d",
			},
		},
	)
	assert.Equal(t, &RosettaTypes.CallResponse{
		Result: map[string]interface{}{
			"blockHash":         "0x928b4d7d1ab8fcb2f62ffa7bba7a1a52251a1145ffc0faec3e009535ba4a2669",
			"blockNumber":       "0x7edcff",
			"contractAddress":   nilAddress,
			"cumulativeGasUsed": "0x744f1b",
			"gasUsed":           "0x5208",
			"logs":              []interface{}{},
			"logsBloom":         "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", // nolint
			"root":              "0x",
			"status":            "0x1",
			"transactionHash":   "0xb358c6958b1cab722752939cbb92e3fec6b6023de360305910ce80c56c3dad9d",
			"transactionIndex":  "0x21",
		},
		Idempotent: false,
	}, resp)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestCall_InvalidArgs(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	resp, err := c.Call(
		ctx,
		&RosettaTypes.CallRequest{
			Method: "eth_getTransactionReceipt",
		},
	)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, ErrCallParametersInvalid))

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestCall_InvalidMethod(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	resp, err := c.Call(
		ctx,
		&RosettaTypes.CallRequest{
			Method: "blah",
		},
	)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, ErrCallMethodInvalid))

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestBlock(t *testing.T) {
	var tests = map[string]struct {
		blockNum               int
		blockNumHexString      string
		useLatest              bool
		author                 string
		blockHash              string
		transactions           []string
		contractAddresses      []string
		partialBlockIdentifier *RosettaTypes.PartialBlockIdentifier
		error                  error
	}{
		"current block (latest)": {
			blockNum:               10992,
			blockNumHexString:      "0x2af0",
			useLatest:              true,
			author:                 nilAddress,
			blockHash:              "0xba9ded5ca1ec9adb9451bf062c9de309d9552fa0f0254a7b982d3daf7ae436ae",
			transactions:           nil,
			partialBlockIdentifier: nil,
			error:                  nil,
		},
		"block by hash (historical)": {
			blockNum:          10992,
			blockNumHexString: "0x2af0",
			useLatest:         false,
			author:            nilAddress,
			blockHash:         "0xba9ded5ca1ec9adb9451bf062c9de309d9552fa0f0254a7b982d3daf7ae436ae",
			transactions:      nil,
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Hash: RosettaTypes.String("0xba9ded5ca1ec9adb9451bf062c9de309d9552fa0f0254a7b982d3daf7ae436ae"),
			},
			error: nil,
		},
		"block by index (historical)": {
			blockNum:          10992,
			blockNumHexString: "0x2af0",
			useLatest:         false,
			author:            nilAddress,
			blockHash:         "0xba9ded5ca1ec9adb9451bf062c9de309d9552fa0f0254a7b982d3daf7ae436ae",
			transactions:      nil,
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(10992)),
			},
			error: nil,
		},
		"block with transaction": {
			blockNum:          10994,
			blockNumHexString: "0x2af2",
			useLatest:         false,
			author:            "0xfFC614eE978630D7fB0C06758DeB580c152154d3",
			blockHash:         "0xb6a2558c2e54bfb11247d0764311143af48d122f29fc408d9519f47d70aa2d50",
			transactions: []string{
				"0xd83b1dcf7d47c4115d78ce0361587604e8157591b118bd64ada02e86c9d5ca7e",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(10994)),
			},
			error: nil,
		},
		"block with three ERC20 transfer transactions": {
			blockNum:          19594674,
			blockNumHexString: "0x12afdb2",
			useLatest:         false,
			author:            "0xE4b8e9222704401aD16d4d826732953DAf07C7E2",
			blockHash:         "0x80714362cc0b6ce164de611c6383c63c3783a9e0f6445e7c6e87b92159ec09ce",
			// 3 sample transactions corresponding to the following contract addresses:
			// 0xf6ab4ba2c101ac9b120d6b9aeb211487bbd8058b
			// 0x8bb570731cbd353203caa75478186918146c867b
			// 0xb923b52b60e247e34f9afe6b3fa5accbaea829e8
			transactions: []string{
				"0x99c0da17e68d1ed46f0b65ed177c7497bf1a8e151c22daf37f93c83ea3c73e6c",
				"0x47e4ec105b3c8b59399e44dd42dd3faa6a2b6f29d034731cb99798f959fc4168",
				"0x204a9205b52a2e8441521796881986066de35eb422c0d8994cc47b3635891f39",
			},
			contractAddresses: []string{
				"0xF6ab4bA2c101aC9b120D6B9AEb211487bbd8058b",
				"0x8Bb570731CBD353203CaA75478186918146c867B",
				"0xB923b52b60E247E34f9afE6B3fa5aCcBAea829E8",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(19594674)),
			},
			error: nil,
		},
		"block with three transactions, one containing multiple ERC20 transfers (of the same token)": {
			blockNum:          20062432,
			blockNumHexString: "0x13220e0",
			useLatest:         false,
			author:            "0xc275DC8bE39f50D12F66B6a63629C39dA5BAe5bd",
			blockHash:         "0xfa80cb6a745c5f98f812626876c7de54816db318c5d090c38bd99905812f5450",
			// 3 sample transactions; 2 correspond with the following contract addresses (the 3rd is a transfer)
			// 0x51eFAbE1718be9940c5f4D10F1983a46d99EC4D9
			// 0x5da92e52854F12eC7D8E32Fba7a573473D648D18 ; contains multiple ERC20 transfers
			transactions: []string{
				"0x51ca5378b15fb13cb6951da8dd681eba5b58497ff9a5021ef089530126bf6b9e",
				"0x83322a654b3a0ba28685ea20b3b33cef2950d05c3335b91792f058df90f10ce6",
				"0x82acfc15f7dcd1caa4c258e924a347add2db8e781ad0435e703e6666d0c37a25",
			},
			// Token addresses
			contractAddresses: []string{
				"0xAA3aE75e8118FC1b6DeBC99Bc52dB28F7403A54c",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(20062432)),
			},
			error: nil,
		},
		"block with partially successful transaction": {
			blockNum:          239782,
			blockNumHexString: "0x3a8a6",
			useLatest:         false,
			author:            "0xe9fB1e9B0D782f6ef112Ad3A4c9E39Dfc13754aC",
			blockHash:         "0xc4487850a40d85b79cf5e5b69db38284fbd39efcf902ca8a6d9f2ba89c538ea3",
			transactions: []string{
				"0x05613760334d347e771fad61b1815c8c817b8dd5f0fcbba57c3f2df67dec33d6",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(239782)),
			},
			error: nil,
		},
		"block with transfer to destroyed contract": {
			blockNum:          363415,
			blockNumHexString: "0x58b97",
			useLatest:         false,
			author:            "0x93Cb12397d6BEa4BfDC03a4B6E33c16aC1b15638",
			blockHash:         "0xf0445269b02ba461af662d8c6aac50d9557a0cc9dbe580d3e180efd7879cc79e",
			transactions: []string{
				"0x9e0f7c64a5bf1fc9f3d7b7963cf23f74e3d2c0b2b3f35f26df031954e5581179",
				"0x0046a7c3ca126864a3e851235ca6bf030300f9138f035f5f190e59ff9a4b22ff",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(363415)),
			},
			error: nil,
		},
		"block with Test ERC20 state sync (no MATIC change)": {
			blockNum:          135808,
			blockNumHexString: "0x21280",
			useLatest:         false,
			author:            "0xC26880A0AF2EA0c7E8130e6EC47Af756465452E8",
			blockHash:         "0xda45a0d75e06aacf58a5a359f7c011e9d17254a0e7f95d99d373ef52b7da3f3d",
			transactions: []string{
				"0xf658e64d2c7dd6a2e32d48c582bbf9db0c45c95db72b4ddd23fe73764849dbdc",
			},
			contractAddresses: []string{
				"0x2d7882beDcbfDDce29Ba99965dd3cdF7fcB10A1e",
			},
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(135808)),
			},
			error: nil,
		},
		"block with MATIC deposit state sync": {
			blockNum:          147648,
			blockNumHexString: "0x240c0",
			useLatest:         false,
			author:            "0xC26880A0AF2EA0c7E8130e6EC47Af756465452E8",
			blockHash:         "0xcbce8f3ca9545917f8c675fbf1723685ce3e18f8ce62ab0893aee813a12d6d27",
			transactions: []string{
				"0x3f4a5a9cbd755b65c85954650abc7c2c39331311bf8490b8ed42c1e0349f65a3",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(147648)),
			},
			error: nil,
		},
		"block with transfer to precompiled": {
			blockNum:          363753,
			blockNumHexString: "0x58ce9",
			useLatest:         false,
			author:            "0x817562f86cEE143236962249453AE54E2b530140",
			blockHash:         "0x3defb56cc49cf7603e08749516a003baae0944596e4555b0d868ec225ff2bcd3",
			transactions: []string{
				"0x586d0a158f29da3d0e8fa4d24596d1a9f6ded03b5ccdb68f40e9372980488fc8",
				"0x80fb7e6bfa8dae67cf79f21b9e68c5af727ba52f3ab1e5a5be5c8048a9758f56",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(363753)),
			},
			error: nil,
		},
		"block with complex self-destruct": {
			blockNum:          468179,
			blockNumHexString: "0x724d3",
			useLatest:         false,
			author:            "0x01711853335F857442eF6f349B2467C531731318",
			blockHash:         "0xd88e8376ec3eef899d9fbc6349e8330ebfc102b245fef784a999ac854091cb64",
			transactions: []string{
				"0x712f7aed1ac12f8a38b4caefea8e7c1940c88add78e110b194c653c9efb3a75d",
				"0x99b723ac54002b16049143474d80f8e6358d14dec2250d873511d091de74977d",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(468179)),
			},
			error: nil,
		},
		"block with complex resurrection": {
			blockNum:          363366,
			blockNumHexString: "0x58b66",
			useLatest:         false,
			author:            "0x3D6F8823Ad21CD299814B62D198d9001E67E20B3",
			blockHash:         "0x5f7c67c2eb0e828b0f4a0e64d5fbae0ed66b70c9ae752e6175c9ef62402502df",
			transactions: []string{
				"0x3f11ca203c7fd814751725c2c5a3efa00bebbbd5e89f406a28b4a36559393b6f",
				"0x4cc86d845b6ee5c12db00cc75c42e98f8bbf62060bc925942c5ff6a36878549b",
				"0xf8b84ff00db596c9db15de1a44c939cce36c0dfd60ef6171db6951b11d7d015d",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(363366)),
			},
			error: nil,
		},
		"block with blackholed funds": {
			blockNum:          468194,
			blockNumHexString: "0x724e2",
			useLatest:         false,
			author:            "0x01711853335F857442eF6f349B2467C531731318",
			blockHash:         "0xf0d9ab47473e38f98b195ba7a17934f68519168f5fdec9899b3c18180d8fbb54",
			transactions: []string{
				"0xbd54f0c5742a5c96ffb358680b88a0f6cfbf83d599dbd0b8fff66b59ed0d7f81",
				"0xf3626ec6a7aba22137b012e8e68513dcaf8574d0412b97e4381513a3ca9ecfc0",
			},
			contractAddresses: nil,
			partialBlockIdentifier: &RosettaTypes.PartialBlockIdentifier{
				Index: RosettaTypes.Int64(int64(468194)),
			},
			error: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mockClient, err := createMockClient(ctx, t)
			assert.NoError(t, err)

			// If we're checking the latest block, we should mock that specific block (with a "latest" identifier).
			// Otherwise, we need to make additional checks before deciding.
			if test.useLatest {
				mockClient.mockGetBlockByNumberLatest(test.blockNum)
			} else {
				// Determine which mock to use, depending on whether a block number or block hash is provided
				if test.partialBlockIdentifier.Hash != nil {
					mockClient.mockGetBlockByHash(test.blockNum, test.blockHash)
				} else {
					mockClient.mockGetBlockByNumber(test.blockNum, test.blockNumHexString)
				}
			}

			mockClient.mockGetAuthor(test.blockNumHexString, test.author)
			mockClient.mockTraceBlockByHash(test.blockHash)

			if test.transactions != nil {
				mockClient.mockGetTransactionReceipts(test.transactions)
			}

			if test.contractAddresses != nil {
				mockClient.mockTokenDetails(test.contractAddresses)
			}

			correctRaw, err := ioutil.ReadFile(fmt.Sprintf("testdata/block/block_response_%d.json", test.blockNum))
			assert.NoError(t, err)
			var correct *RosettaTypes.BlockResponse
			assert.NoError(t, json.Unmarshal(correctRaw, &correct))

			resp, err := mockClient.client.Block(
				ctx,
				test.partialBlockIdentifier,
			)
			assert.NoError(t, err)

			// Ensure types match
			jsonResp, err := jsonifyBlock(resp)
			assert.Equal(t, correct.Block, jsonResp)
			assert.Equal(t, test.error, err)

			mockClient.jsonRPC.AssertExpectations(t)
			mockClient.graphQL.AssertExpectations(t)
		})
	}
}

func TestPendingNonceAt(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_getTransactionCount",
		common.HexToAddress("0xfFC614eE978630D7fB0C06758DeB580c152154d3"),
		"pending",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*hexutil.Uint64)

			*r = hexutil.Uint64(10)
		},
	).Once()
	resp, err := c.PendingNonceAt(
		ctx,
		common.HexToAddress("0xfFC614eE978630D7fB0C06758DeB580c152154d3"),
	)
	assert.Equal(t, uint64(10), resp)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestSuggestGasPrice_Nil(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_gasPrice",
	).Return(
		nil,
	).Run(
		func(args mock.Arguments) {
			r := args.Get(1).(*hexutil.Big)

			*r = *(*hexutil.Big)(big.NewInt(100000))
		},
	).Once()
	resp, err := c.SuggestGasPrice(
		ctx,
		nil,
	)
	assert.Equal(t, big.NewInt(100000), resp)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestSuggestGasPrice_Valid(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	gasPrice, _ := new(big.Int).SetString("100000000000", 10)
	resp, err := c.SuggestGasPrice(
		ctx,
		gasPrice,
	)
	assert.Equal(t, gasPrice, resp)
	assert.NoError(t, err)

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestSendTransaction(t *testing.T) {
	mockJSONRPC := &mocks.JSONRPC{}
	mockGraphQL := &mocks.GraphQL{}

	cf, err := newERC20CurrencyFetcher(mockGraphQL)
	assert.NoError(t, err)

	c := &Client{
		c:               mockJSONRPC,
		g:               mockGraphQL,
		currencyFetcher: cf,
		traceSemaphore:  semaphore.NewWeighted(100),
	}

	ctx := context.Background()
	mockJSONRPC.On(
		"CallContext",
		ctx,
		mock.Anything,
		"eth_sendRawTransaction",
		"0xf86a80843b9aca00825208941ff502f9fe838cd772874cb67d0d96b93fd1d6d78725d4b6199a415d8029a01d110bf9fd468f7d00b3ce530832e99818835f45e9b08c66f8d9722264bb36c7a02711f47ec99f9ac585840daef41b7118b52ec72f02fcb30d874d36b10b668b59", // nolint
	).Return(
		nil,
	).Once()

	rawTx, err := ioutil.ReadFile("testdata/submitted_tx.json")
	assert.NoError(t, err)

	tx := new(types.Transaction)
	assert.NoError(t, tx.UnmarshalJSON(rawTx))

	assert.NoError(t, c.SendTransaction(
		ctx,
		tx,
	))

	mockJSONRPC.AssertExpectations(t)
	mockGraphQL.AssertExpectations(t)
}

func TestDecodeHexData(t *testing.T) {
	hexWithoutLeadingZeroes := "0x2324c0d180077fe7000"
	bal, _ := new(big.Int).SetString("10372550232136640000000", 10)
	decodedBal, err := decodeHexData(hexWithoutLeadingZeroes)
	assert.NoError(t, err)
	assert.Equal(t, decodedBal, bal)

	hexWithLeadingZeroes := "0x00000000000000000000000000000000000000000000021e19e0c9bab2400000"
	bal, _ = new(big.Int).SetString("10000000000000000000000", 10)
	decodedBal, err = decodeHexData(hexWithLeadingZeroes)
	assert.NoError(t, err)
	assert.Equal(t, decodedBal, bal)
}
