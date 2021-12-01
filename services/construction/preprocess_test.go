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

package construction

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/maticnetwork/polygon-rosetta/polygon"
	svcErrors "github.com/maticnetwork/polygon-rosetta/services/errors"
	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/stretchr/testify/assert"
)

var (
	preprocessFromAddress          = fromAddress
	preprocessToAddress            = toAddress
	preprocessTokenContractAddress = tokenContractAddress

	preprocessTransferValue    = uint64(1)
	preprocessTransferValueHex = hexutil.EncodeUint64(preprocessTransferValue)
	preprocessData             = "0xa9059cbb000000000000000000000000efd3dc58d60af3295b92ecd484caeb3a2f30b3e70000000000000000000000000000000000000000000000000000000000000001" // nolint
	preprocessGasPrice         = uint64(100000000000)
	preprocessGasPriceHex      = hexutil.EncodeUint64(preprocessGasPrice)
)

func TestPreprocess(t *testing.T) {
	tests := map[string]struct {
		operations []*types.Operation
		metadata   map[string]interface{}

		expectedResponse *types.ConstructionPreprocessResponse
		expectedError    *types.Error
	}{
		"happy path: native currency": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":  preprocessFromAddress,
					"to":    preprocessToAddress,
					"value": preprocessTransferValueHex,
				},
			},
		},
		"happy path: ERC20 currency": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
				},
			},
		},
		"happy path: native currency with nonce": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce": "1",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":  preprocessFromAddress,
					"to":    preprocessToAddress,
					"value": preprocessTransferValueHex,
					"nonce": "0x1",
				},
			},
		},
		"happy path: ERC20 currency with nonce": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"nonce": "34",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"nonce":         "0x22",
				},
			},
		},
		"happy path: native currency with gas price": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_price": "100000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":      preprocessFromAddress,
					"to":        preprocessToAddress,
					"value":     preprocessTransferValueHex,
					"gas_price": preprocessGasPriceHex,
				},
			},
		},
		"happy path: ERC20 currency with gas price": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"gas_price": "100000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"gas_price":     preprocessGasPriceHex,
				},
			},
		},
		"error: both positive amount": {
			operations: func() []*types.Operation {
				operations := templateOperations(preprocessTransferValue, polygon.Currency)
				operations[0].Amount.Value = "1"
				return operations
			}(),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrUnclearIntent, "unable to find match for operation: at index 1"),
		},
		"error: missing currency": {
			operations: func() []*types.Operation {
				operations := templateOperations(preprocessTransferValue, polygon.Currency)
				operations[0].Amount.Currency = nil
				return operations
			}(),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrUnclearIntent, "missing currency"),
		},
		"error: unequal currency": {
			operations: func() []*types.Operation {
				operations := templateOperations(preprocessTransferValue, polygon.Currency)
				operations[0].Amount.Currency = &types.Currency{
					Symbol:   "USDC",
					Decimals: 18,
				}
				return operations
			}(),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrUnclearIntent, "from and to currencies are not equal"),
		},
		"error: invalid from address": {
			operations: func() []*types.Operation {
				operations := templateOperations(preprocessTransferValue, polygon.Currency)
				operations[0].Account.Address = "invalid"
				return operations
			}(),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidAddress, "source address: invalid is not a valid address"),
		},
		"error: invalid destination address": {
			operations: func() []*types.Operation {
				operations := templateOperations(preprocessTransferValue, polygon.Currency)
				operations[1].Account.Address = "invalid"
				return operations
			}(),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidAddress, "destination address: invalid is not a valid address"),
		},
		"error: invalid nonce string": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce": map[string]string{},
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidNonce, "map[] is not a valid nonce string"),
		},
		"error: invalid nonce": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce": "invalid_nonce",
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidNonce, "invalid_nonce is not a valid nonce"),
		},
		"error: missing token address": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
			}),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidTokenContractAddress, "missing token contract address"),
		},
		"error: token address not a string": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": map[string]string{},
				},
			}),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidTokenContractAddress, "token contract address is not a string"),
		},
		"error: token address invalid": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": "invalid_token_address",
				},
			}),
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidTokenContractAddress, "token contract address is not a valid address"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			service := APIService{}
			request := &types.ConstructionPreprocessRequest{
				NetworkIdentifier: networkIdentifier,
				Operations:        test.operations,
				Metadata:          test.metadata,
			}
			resp, err := service.ConstructionPreprocess(context.Background(), request)

			if err != nil {
				assert.Equal(t, test.expectedError, err)
			} else {
				assert.Equal(t, test.expectedResponse, resp)
			}
		})
	}
}
