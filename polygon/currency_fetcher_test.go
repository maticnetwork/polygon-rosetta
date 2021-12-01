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
	"fmt"
	"io/ioutil"
	"testing"

	mocks "github.com/maticnetwork/polygon-rosetta/mocks/polygon"
	"github.com/stretchr/testify/assert"

	RosettaTypes "github.com/coinbase/rosetta-sdk-go/types"
)

const (
	MOTContractAddress = "0xAA3aE75e8118FC1b6DeBC99Bc52dB28F7403A54c"
	MOTSymbol          = "MOT"
	MOTDecimals        = 18

	// emptySymbolContractAddress has *no* symbol; Polygonscan displays it as "N/A"
	emptySymbolContractAddress = "0xE5961bDFc48023f9f02E2d05b1115fB1e5695B08"
	emptySymbolDecimals        = 0

	blankSymbol                = ""
	blankSymbolContractAddress = "0x6bd4e69abe087be7c09f094087d5c7f75b010abc"
	blankSymbolDecimals        = 18

	invalidContractAddressNonHex        = "0xdeadbeefdeadbeefdeadbeefdeadbeefzzzzzzzz"
	invalidContractAddressTooShort      = "0xdeadbeef"
	invalidContractAddressMissingPrefix = "deadbeef"

	unknownContractAddress = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

var MOTCurrency = &RosettaTypes.Currency{
	Symbol:   MOTSymbol,
	Decimals: int32(MOTDecimals),
	Metadata: map[string]interface{}{
		ContractAddressKey: MOTContractAddress,
	},
}

var blankSymbolCurrency = &RosettaTypes.Currency{
	Symbol:   defaultERC20Symbol,
	Decimals: int32(blankSymbolDecimals),
	Metadata: map[string]interface{}{
		ContractAddressKey: blankSymbolContractAddress,
	},
}

var emptySymbolCurrency = &RosettaTypes.Currency{
	Symbol:   defaultERC20Symbol,
	Decimals: int32(emptySymbolDecimals),
	Metadata: map[string]interface{}{
		ContractAddressKey: emptySymbolContractAddress,
	},
}

var unknownCurrency = &RosettaTypes.Currency{
	Symbol:   defaultERC20Symbol,
	Decimals: int32(defaultERC20Decimals),
	Metadata: map[string]interface{}{
		ContractAddressKey: unknownContractAddress,
	},
}

func mockFetchSymbol(
	t *testing.T,
	mockGraphQL *mocks.GraphQL,
	contractAddress string,
) {
	ctx := context.Background()

	// Mock symbol call
	result, err := ioutil.ReadFile(fmt.Sprintf("testdata/token_contracts/symbol_%s.json", contractAddress))
	assert.NoError(t, err)
	mockGraphQL.On(
		"Query",
		ctx,
		buildGraphqlCallQuery("", contractAddress, symbolABIEncoded),
	).Return(
		string(result),
		nil,
	).Once()
}

func mockFetchDecimals(
	t *testing.T,
	mockGraphQL *mocks.GraphQL,
	contractAddress string,
) {
	ctx := context.Background()

	// Mock decimals call
	result, err := ioutil.ReadFile(fmt.Sprintf("testdata/token_contracts/decimals_%s.json", contractAddress))
	assert.NoError(t, err)
	mockGraphQL.On(
		"Query",
		ctx,
		buildGraphqlCallQuery("", contractAddress, decimalsABIEncoded),
	).Return(
		string(result),
		nil,
	).Once()
}

func TestFetchCurrency(t *testing.T) {
	var tests = map[string]struct {
		contractAddress  string
		expectedCurrency *RosettaTypes.Currency
		decimals         int
		symbol           string
		mockFn           func(mockGraphQL *mocks.GraphQL, contractAddress string)
		error            error
	}{
		"happy path: valid contract address": {
			contractAddress:  MOTContractAddress,
			expectedCurrency: MOTCurrency,
			decimals:         MOTDecimals,
			symbol:           MOTSymbol,
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
				mockFetchSymbol(t, mockGraphQL, contractAddress)
			},
			error: nil,
		},
		"happy path: invalid currency details parsed as unknownCurrency": {
			contractAddress:  unknownContractAddress,
			expectedCurrency: unknownCurrency,
			decimals:         defaultERC20Decimals,
			symbol:           defaultERC20Symbol,
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
				mockFetchSymbol(t, mockGraphQL, contractAddress)
			},
			error: nil,
		},
		"happy path: empty symbol parsed as UNKNOWN": {
			contractAddress:  emptySymbolContractAddress,
			expectedCurrency: emptySymbolCurrency,
			decimals:         emptySymbolDecimals,
			symbol:           defaultERC20Symbol,
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
				mockFetchSymbol(t, mockGraphQL, contractAddress)
			},
			error: nil,
		},
		"invalid currency: successful call to fetch decimals, unsuccessful call to fetch symbol": {
			contractAddress:  blankSymbolContractAddress,
			expectedCurrency: blankSymbolCurrency,
			decimals:         blankSymbolDecimals,
			symbol:           defaultERC20Symbol,
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
				mockFetchSymbol(t, mockGraphQL, contractAddress)
			},
			error: nil,
		},
		"invalid contract address: non-hex": {
			contractAddress:  invalidContractAddressNonHex,
			expectedCurrency: nil,
			decimals:         0,
			symbol:           blankSymbol,
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
			},
			error: fmt.Errorf("[{\"message\":\"invalid hex string\",\"path\":null}]"),
		},
		"invalid contract address: too short": {
			contractAddress:  invalidContractAddressTooShort,
			expectedCurrency: nil,
			decimals:         0,
			symbol:           "",
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
			},
			error: fmt.Errorf("[{\"message\":\"hex string has length 8, want 40 for Address\",\"path\":null}]"), //nolint
		},
		"invalid contract address: missing 0x prefix": {
			contractAddress:  invalidContractAddressMissingPrefix,
			expectedCurrency: nil,
			decimals:         0,
			symbol:           "",
			mockFn: func(mockGraphQL *mocks.GraphQL, contractAddress string) {
				mockFetchDecimals(t, mockGraphQL, contractAddress)
			},
			error: fmt.Errorf("[{\"message\":\"hex string without 0x prefix\",\"path\":null}]"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mockGraphQL := &mocks.GraphQL{}

			test.mockFn(mockGraphQL, test.contractAddress)

			cf, err := newERC20CurrencyFetcher(mockGraphQL)
			assert.NoError(t, err)

			fetchedCurrency, err := cf.fetchCurrency(ctx, test.contractAddress)
			assert.Equal(t, test.expectedCurrency, fetchedCurrency)
			assert.Equal(t, test.error, err)

			mockGraphQL.AssertExpectations(t)

			// Currencies for which we were able to successfully fetch details should be cached,
			// such that subsequent queries are not needed (hence we don't need to re-mock)
			if test.error == nil {
				fetchedCurrency, err = cf.fetchCurrency(ctx, test.contractAddress)
				assert.Equal(t, test.expectedCurrency, fetchedCurrency)
				assert.NoError(t, err)

				mockGraphQL.AssertExpectations(t)
			}
		})
	}
}
