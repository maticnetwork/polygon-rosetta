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

	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/maticnetwork/polygon-rosetta/polygon"
	svcErrors "github.com/maticnetwork/polygon-rosetta/services/errors"
	"github.com/stretchr/testify/assert"
)

var (
	preprocessFromAddress             = fromAddress
	preprocessToAddress               = toAddress
	preprocessTokenContractAddress    = tokenContractAddress
	preprocessZeroTransferValue       = uint64(0)
	preprocessTransferValue           = uint64(1)
	preprocessTransferValueLargeValue = uint64(100000000000)
	preprocessTransferValueHex        = hexutil.EncodeUint64(preprocessTransferValue)
	preprocessTransferValueLargeHex   = hexutil.EncodeUint64(preprocessTransferValueLargeValue)
	preprocessData                    = "0xa9059cbb000000000000000000000000efd3dc58d60af3295b92ecd484caeb3a2f30b3e70000000000000000000000000000000000000000000000000000000000000001" // nolint
	preprocessGasLimit                = uint64(600000)
	preprocessGasLimitHex             = hexutil.EncodeUint64(preprocessGasLimit)
	preprocessGasTip                  = uint64(20000000)
	preprocessGasTipHex               = hexutil.EncodeUint64(preprocessGasTip)
	preprocessGasCap                  = uint64(5000000000)
	preprocessGasCapHex               = hexutil.EncodeUint64(preprocessGasCap)
	preprocessGenericData             = "0x095ea7b3000000000000000000000000d10a72cf054650931365cc44d912a4fd7525705800000000000000000000000000000000000000000000000000000000000003e8"
	methodSignature                   = "approve(address,uint256)"
	methodArgs                        = []string{"0xD10a72Cf054650931365Cc44D912a4FD75257058", "1000"}
	expectedMethodArgs                = []interface{}{"0xD10a72Cf054650931365Cc44D912a4FD75257058", "1000"}
	complexMethodSignature            = "mintItemBatch(address[],string)"
	complexMethodArgs                 = "000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000e00000000000000000000000000000000000000000000000000000000000000004000000000000000000000000b406c0106ba32281ddfa75626479304feb70d0580000000000000000000000003cdc2ce790d740fd8b8e99baf738497c5e2de62000000000000000000000000006da92f4f1815e83cf5a020f952f0e3275a5b156000000000000000000000000f344767634735d588357ed5828488094bef02efe000000000000000000000000000000000000000000000000000000000000002e516d614b57483933397346454464576333347252395453433868647758624357574575454a6b6476714e334a7573000000000000000000000000000000000000"
	complexMethodData                 = "0x079c66c0000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000e00000000000000000000000000000000000000000000000000000000000000004000000000000000000000000b406c0106ba32281ddfa75626479304feb70d0580000000000000000000000003cdc2ce790d740fd8b8e99baf738497c5e2de62000000000000000000000000006da92f4f1815e83cf5a020f952f0e3275a5b156000000000000000000000000f344767634735d588357ed5828488094bef02efe000000000000000000000000000000000000000000000000000000000000002e516d614b57483933397346454464576333347252395453433868647758624357574575454a6b6476714e334a7573000000000000000000000000000000000000"
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
		"happy path: native currency with large amount": {
			operations: templateOperations(preprocessTransferValueLargeValue, polygon.Currency),
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":  preprocessFromAddress,
					"to":    preprocessToAddress,
					"value": preprocessTransferValueLargeHex,
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
		"happy path: native currency with nonce and gas_tip": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce": "1",
				"gas_tip": "40000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":  preprocessFromAddress,
					"to":    preprocessToAddress,
					"value": preprocessTransferValueHex,
					"nonce": "0x1",
					"gas_tip": transferGasTipHex, // hex of 40000000000
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
		"happy path: ERC20 currency with nonce and gas_tip": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"nonce": "34",
				"gas_tip": "40000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"nonce":         "0x22",
					"gas_tip": transferGasTipHex, // hex of 40000000000
				},
			},
		},
		"happy path: Generic Contract call": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce":            "34",
				"method_signature": methodSignature,
				"method_args":      methodArgs,
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":             preprocessFromAddress,
					"to":               preprocessToAddress, // it will be contract address user need to pass in operation
					"value":            "0x1",
					"contract_address": preprocessToAddress,
					"data":             preprocessGenericData,
					"nonce":            "0x22",
					"method_signature": methodSignature,
					"method_args":      expectedMethodArgs,
				},
			},
		},
		"happy path: Generic Contract call with zero transfer value": {
			operations: templateOperations(preprocessZeroTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce":            "34",
				"method_signature": "approve(address,uint256)",
				"method_args":      []string{"0xD10a72Cf054650931365Cc44D912a4FD75257058", "1000"},
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":             preprocessFromAddress,
					"to":               preprocessToAddress, // it will be contract address user need to pass in operation
					"value":            "0x0",
					"contract_address": preprocessToAddress,
					"data":             preprocessGenericData,
					"nonce":            "0x22",
					"method_signature": methodSignature,
					"method_args":      expectedMethodArgs,
				},
			},
		},
		"happy path: generic contract call with pre-encoded arguments": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"nonce":            "34",
				"method_signature": complexMethodSignature,
				"method_args":      complexMethodArgs,
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":             preprocessFromAddress,
					"to":               preprocessToAddress,
					"value":            "0x1",
					"contract_address": preprocessToAddress,
					"data":             complexMethodData,
					"nonce":            "0x22",
					"method_signature": complexMethodSignature,
					"method_args":      complexMethodArgs,
				},
			},
		},
		"happy path: native currency with gas limit": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_limit": "600000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":      preprocessFromAddress,
					"to":        preprocessToAddress,
					"value":     preprocessTransferValueHex,
					"gas_limit": preprocessGasLimitHex,
				},
			},
		},
		"happy path: ERC20 currency with gas limit": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"gas_limit": "600000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"gas_limit":     preprocessGasLimitHex,
				},
			},
		},
		"happy path: native currency with gas cap": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_cap": "5000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":    preprocessFromAddress,
					"to":      preprocessToAddress,
					"value":   preprocessTransferValueHex,
					"gas_cap": preprocessGasCapHex,
				},
			},
		},
		"happy path: ERC20 currency with gas cap": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"gas_cap": "5000000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"gas_cap":       preprocessGasCapHex,
				},
			},
		},
		"happy path: native currency with gas tip": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_tip": "20000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":    preprocessFromAddress,
					"to":      preprocessToAddress,
					"value":   preprocessTransferValueHex,
					"gas_tip": preprocessGasTipHex,
				},
			},
		},
		"happy path: ERC20 currency with gas tip": {
			operations: templateOperations(preprocessTransferValue, &types.Currency{
				Symbol:   "USDC",
				Decimals: 18,
				Metadata: map[string]interface{}{
					"token_address": preprocessTokenContractAddress,
				},
			}),
			metadata: map[string]interface{}{
				"gas_tip": "20000000",
			},
			expectedResponse: &types.ConstructionPreprocessResponse{
				Options: map[string]interface{}{
					"from":          preprocessFromAddress,
					"to":            preprocessToAddress,
					"value":         "0x0",
					"token_address": preprocessTokenContractAddress,
					"data":          preprocessData,
					"gas_tip":       preprocessGasTipHex,
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
		"error: invalid gas limit string": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_limit": map[string]string{},
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasLimit, "map[] is not a valid gas_limit string"),
		},
		"error: invalid gas limit": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_limit": "gas_limit",
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasLimit, "gas_limit is not a valid gas_limit"),
		},
		"error: invalid gas cap string": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_cap": map[string]string{},
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasCap, "map[] is not a valid gas_cap string"),
		},
		"error: invalid gas cap": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_cap": "gas_cap",
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasCap, "gas_cap is not a valid gas_cap"),
		},
		"error: invalid gas tip string": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_tip": map[string]string{},
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasTip, "map[] is not a valid gas_tip string"),
		},
		"error: invalid gas tip": {
			operations: templateOperations(preprocessTransferValue, polygon.Currency),
			metadata: map[string]interface{}{
				"gas_tip": "gas_tip",
			},
			expectedResponse: nil,
			expectedError: templateError(
				svcErrors.ErrInvalidGasTip, "gas_tip is not a valid gas_tip"),
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
