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

package construction

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/params"

	"github.com/maticnetwork/polygon-rosetta/configuration"
	mocks "github.com/maticnetwork/polygon-rosetta/mocks/services"
	"github.com/maticnetwork/polygon-rosetta/polygon"

	"github.com/coinbase/rosetta-sdk-go/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	networkIdentifier = &types.NetworkIdentifier{
		Network:    polygon.TestnetNetwork,
		Blockchain: polygon.Blockchain,
	}

	chainID    = uint64(80001)
	chainIDHex = hexutil.EncodeUint64(chainID)

	fromAddress          = "0x966fbC4E1F3a938Cf7798695C3244d9C7C190015"
	toAddress            = "0xefD3dc58D60aF3295B92ecd484CAEB3A2f30b3e7"
	tokenContractAddress = "0x2d7882beDcbfDDce29Ba99965dd3cdF7fcB10A1e"

	transferValue         = uint64(20211004)
	transferGasPrice      = uint64(5000000000)
	transferGasLimit      = uint64(21000)
	transferGasLimitERC20 = uint64(65000)
	transferNonce         = uint64(67)
	transferData          = "0xa9059cbb000000000000000000000000efd3dc58d60af3295b92ecd484caeb3a2f30b3e7000000000000000000000000000000000000000000000000000000000134653c" //nolint

	transferValueHex         = hexutil.EncodeUint64(transferValue)
	transferGasPriceHex      = hexutil.EncodeUint64(transferGasPrice)
	transferGasLimitHex      = hexutil.EncodeUint64(transferGasLimit)
	transferGasLimitERC20Hex = hexutil.EncodeUint64(transferGasLimitERC20)
	transferNonceHex         = hexutil.EncodeUint64(transferNonce)
	transferNonceHex2        = "0x22"
)

func forceHexDecode(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("could not decode hex %s", s)
	}

	return b
}

func forceMarshalMap(t *testing.T, i interface{}) map[string]interface{} {
	m, err := marshalJSONMap(i)
	if err != nil {
		t.Fatalf("could not marshal map %s", types.PrintStruct(i))
	}

	return m
}

func TestConstructionFlowWithPendingNonce(t *testing.T) {
	cfg := &configuration.Configuration{
		Mode:    configuration.Online,
		Network: networkIdentifier,
		Params:  params.GoerliChainConfig,
	}
	cfg.Params.ChainID.SetString(configuration.MumbaiChainID, 10)

	mockClient := &mocks.Client{}
	servicer := NewAPIService(cfg, mockClient)
	ctx := context.Background()

	// Test Derive
	publicKey := &types.PublicKey{
		Bytes: forceHexDecode(
			t,
			"0212e9f98d9750e5f74b4b4b00df39074f86c79187943bdb3c5a9c89ffc1ed0188",
		),
		CurveType: types.Secp256k1,
	}
	deriveResponse, err := servicer.ConstructionDerive(ctx, &types.ConstructionDeriveRequest{
		NetworkIdentifier: networkIdentifier,
		PublicKey:         publicKey,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionDeriveResponse{
		AccountIdentifier: &types.AccountIdentifier{
			Address: "0xD10a72Cf054650931365Cc44D912a4FD75257058",
		},
	}, deriveResponse)

	// Test Preprocess
	intent := `[{"operation_identifier":{"index":0},"type":"CALL","account":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"amount":{"value":"-42894881044106498","currency":{"symbol":"MATIC","decimals":18}}},{"operation_identifier":{"index":1},"type":"CALL","account":{"address":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d"},"amount":{"value":"42894881044106498","currency":{"symbol":"MATIC","decimals":18}}}]` // nolint
	var ops []*types.Operation
	assert.NoError(t, json.Unmarshal([]byte(intent), &ops))
	preprocessResponse, err := servicer.ConstructionPreprocess(
		ctx,
		&types.ConstructionPreprocessRequest{
			NetworkIdentifier: networkIdentifier,
			Operations:        ops,
		},
	)
	assert.Nil(t, err)
	optionsRaw := `{"from":"0xD10a72Cf054650931365Cc44D912a4FD75257058", "to": "0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d", "value":"0x9864aac3510d02"}` //nolint
	var options options
	assert.NoError(t, json.Unmarshal([]byte(optionsRaw), &options))
	assert.Equal(t, &types.ConstructionPreprocessResponse{
		Options: forceMarshalMap(t, &options),
	}, preprocessResponse)

	// Test Metadata
	metadata := &metadata{
		GasLimit: big.NewInt(21000),
		GasPrice: big.NewInt(1000000000),
		Nonce:    0,
		To:       "0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d",
		Value:    big.NewInt(42894881044106498),
	}

	var gasPrice *big.Int = nil

	mockClient.On(
		"SuggestGasPrice",
		ctx,
		gasPrice,
	).Return(
		big.NewInt(1000000000),
		nil,
	).Once()
	mockClient.On(
		"PendingNonceAt",
		ctx,
		common.HexToAddress("0xD10a72Cf054650931365Cc44D912a4FD75257058"),
	).Return(
		uint64(0),
		nil,
	).Once()
	metadataResponse, err := servicer.ConstructionMetadata(ctx, &types.ConstructionMetadataRequest{
		NetworkIdentifier: networkIdentifier,
		Options:           forceMarshalMap(t, &options),
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionMetadataResponse{
		Metadata: forceMarshalMap(t, metadata),
		SuggestedFee: []*types.Amount{
			{
				Value:    "21000000000000",
				Currency: polygon.Currency,
			},
		},
	}, metadataResponse)

	// Test Payloads
	unsignedRaw := `{"from":"0xD10a72Cf054650931365Cc44D912a4FD75257058","to":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d","value":"0x9864aac3510d02","data":"0x","nonce":"0x0","gas_price":"0x3b9aca00","gas":"0x5208","chain_id":"0x13881"}` // nolint
	payloadsResponse, err := servicer.ConstructionPayloads(ctx, &types.ConstructionPayloadsRequest{
		NetworkIdentifier: networkIdentifier,
		Operations:        ops,
		Metadata:          forceMarshalMap(t, metadata),
	})
	assert.Nil(t, err)
	payloadsRaw := `[{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058","hex_bytes":"375623b2f9164db0bc050c357fb4e6b57a60ffa1eba0161fe12e96384103218c","account_identifier":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"signature_type":"ecdsa_recovery"}]` // nolint
	var payloads []*types.SigningPayload
	assert.NoError(t, json.Unmarshal([]byte(payloadsRaw), &payloads))
	assert.Equal(t, &types.ConstructionPayloadsResponse{
		UnsignedTransaction: unsignedRaw,
		Payloads:            payloads,
	}, payloadsResponse)

	// Test Parse Unsigned
	parseOpsRaw := `[{"operation_identifier":{"index":0},"type":"CALL","account":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"amount":{"value":"-42894881044106498","currency":{"symbol":"MATIC","decimals":18}}},{"operation_identifier":{"index":1},"related_operations":[{"index":0}],"type":"CALL","account":{"address":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d"},"amount":{"value":"42894881044106498","currency":{"symbol":"MATIC","decimals":18}}}]` // nolint
	var parseOps []*types.Operation
	assert.NoError(t, json.Unmarshal([]byte(parseOpsRaw), &parseOps))
	parseUnsignedResponse, err := servicer.ConstructionParse(ctx, &types.ConstructionParseRequest{
		NetworkIdentifier: networkIdentifier,
		Signed:            false,
		Transaction:       unsignedRaw,
	})
	assert.Nil(t, err)
	parseMetadata := &parseMetadata{
		Nonce:    metadata.Nonce,
		GasPrice: metadata.GasPrice,
		GasLimit: metadata.GasLimit.Uint64(),
		ChainID:  big.NewInt(80001),
	}
	assert.Equal(t, &types.ConstructionParseResponse{
		Operations:               parseOps,
		AccountIdentifierSigners: []*types.AccountIdentifier{},
		Metadata:                 forceMarshalMap(t, parseMetadata),
	}, parseUnsignedResponse)

	// Test Combine
	signaturesRaw := `[{"hex_bytes":"303b2ff05024c20f1775dad9a6e8152fa75bec47c051d7fd2e39572fbddd048e00f2c494280dfa0465d384280dc918c930aae0874714e893382c16058aadf50501","signing_payload":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058","hex_bytes":"375623b2f9164db0bc050c357fb4e6b57a60ffa1eba0161fe12e96384103218c","account_identifier":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"signature_type":"ecdsa_recovery"},"public_key":{"hex_bytes":"0212e9f98d9750e5f74b4b4b00df39074f86c79187943bdb3c5a9c89ffc1ed0188","curve_type":"secp256k1"},"signature_type":"ecdsa_recovery"}]` // nolint
	var signatures []*types.Signature
	assert.NoError(t, json.Unmarshal([]byte(signaturesRaw), &signatures))
	signedRaw := `{"nonce":"0x0","gasPrice":"0x3b9aca00","gas":"0x5208","to":"0x57b414a0332b5cab885a451c2a28a07d1e9b8a8d","value":"0x9864aac3510d02","input":"0x","v":"0x27126","r":"0x303b2ff05024c20f1775dad9a6e8152fa75bec47c051d7fd2e39572fbddd048e","s":"0xf2c494280dfa0465d384280dc918c930aae0874714e893382c16058aadf505","hash":"0x2500ef3f8531452210cfdfe3c11111e9605a2acdd260ac75c8c3ade30258228e"}` // nolint
	combineResponse, err := servicer.ConstructionCombine(ctx, &types.ConstructionCombineRequest{
		NetworkIdentifier:   networkIdentifier,
		UnsignedTransaction: unsignedRaw,
		Signatures:          signatures,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionCombineResponse{
		SignedTransaction: signedRaw,
	}, combineResponse)

	// Test Parse Signed
	parseSignedResponse, err := servicer.ConstructionParse(ctx, &types.ConstructionParseRequest{
		NetworkIdentifier: networkIdentifier,
		Signed:            true,
		Transaction:       signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionParseResponse{
		Operations: parseOps,
		AccountIdentifierSigners: []*types.AccountIdentifier{
			{Address: "0xD10a72Cf054650931365Cc44D912a4FD75257058"},
		},
		Metadata: forceMarshalMap(t, parseMetadata),
	}, parseSignedResponse)

	// Test Hash
	transactionIdentifier := &types.TransactionIdentifier{
		Hash: "0x2500ef3f8531452210cfdfe3c11111e9605a2acdd260ac75c8c3ade30258228e",
	}
	hashResponse, err := servicer.ConstructionHash(ctx, &types.ConstructionHashRequest{
		NetworkIdentifier: networkIdentifier,
		SignedTransaction: signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.TransactionIdentifierResponse{
		TransactionIdentifier: transactionIdentifier,
	}, hashResponse)

	// Test Submit
	mockClient.On(
		"SendTransaction",
		ctx,
		mock.Anything, // can't test ethTx here because it contains "time"
	).Return(
		nil,
	)
	submitResponse, err := servicer.ConstructionSubmit(ctx, &types.ConstructionSubmitRequest{
		NetworkIdentifier: networkIdentifier,
		SignedTransaction: signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.TransactionIdentifierResponse{
		TransactionIdentifier: transactionIdentifier,
	}, submitResponse)

	mockClient.AssertExpectations(t)
}

func TestConstructionFlowWithInputNonce(t *testing.T) {
	networkIdentifier = &types.NetworkIdentifier{
		Network:    polygon.TestnetNetwork,
		Blockchain: polygon.Blockchain,
	}

	cfg := &configuration.Configuration{
		Mode:    configuration.Online,
		Network: networkIdentifier,
		Params:  params.GoerliChainConfig,
	}
	cfg.Params.ChainID.SetString(configuration.MumbaiChainID, 10)

	mockClient := &mocks.Client{}
	servicer := NewAPIService(cfg, mockClient)
	ctx := context.Background()

	// Test Derive
	publicKey := &types.PublicKey{
		Bytes: forceHexDecode(
			t,
			"0212e9f98d9750e5f74b4b4b00df39074f86c79187943bdb3c5a9c89ffc1ed0188",
		),
		CurveType: types.Secp256k1,
	}
	deriveResponse, err := servicer.ConstructionDerive(ctx, &types.ConstructionDeriveRequest{
		NetworkIdentifier: networkIdentifier,
		PublicKey:         publicKey,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionDeriveResponse{
		AccountIdentifier: &types.AccountIdentifier{
			Address: "0xD10a72Cf054650931365Cc44D912a4FD75257058",
		},
	}, deriveResponse)

	// Test Preprocess
	intent := `[{"operation_identifier":{"index":0},"type":"CALL","account":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"amount":{"value":"-42894881044106498","currency":{"symbol":"MATIC","decimals":18}}},{"operation_identifier":{"index":1},"type":"CALL","account":{"address":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d"},"amount":{"value":"42894881044106498","currency":{"symbol":"MATIC","decimals":18}}}]` // nolint
	var ops []*types.Operation
	assert.NoError(t, json.Unmarshal([]byte(intent), &ops))
	preprocessResponse, err := servicer.ConstructionPreprocess(
		ctx,
		&types.ConstructionPreprocessRequest{
			NetworkIdentifier: networkIdentifier,
			Operations:        ops,
			Metadata:          map[string]interface{}{"nonce": "1"},
		},
	)
	assert.Nil(t, err)
	optionsRaw := `{"from":"0xD10a72Cf054650931365Cc44D912a4FD75257058", "nonce":"0x1", "to":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d", "value":"0x9864aac3510d02"}` // nolint
	var options options
	assert.NoError(t, json.Unmarshal([]byte(optionsRaw), &options))
	assert.Equal(t, &types.ConstructionPreprocessResponse{
		Options: forceMarshalMap(t, &options),
	}, preprocessResponse)

	// Test Metadata
	metadata := &metadata{
		GasLimit: big.NewInt(21000),
		GasPrice: big.NewInt(1000000000),
		Nonce:    1,
		To:       "0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d",
		Value:    big.NewInt(42894881044106498),
	}

	var gasPrice *big.Int = nil

	mockClient.On(
		"SuggestGasPrice",
		ctx,
		gasPrice,
	).Return(
		big.NewInt(1000000000),
		nil,
	).Once()
	metadataResponse, err := servicer.ConstructionMetadata(ctx, &types.ConstructionMetadataRequest{
		NetworkIdentifier: networkIdentifier,
		Options:           preprocessResponse.Options,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionMetadataResponse{
		Metadata: forceMarshalMap(t, metadata),
		SuggestedFee: []*types.Amount{
			{
				Value:    "21000000000000",
				Currency: polygon.Currency,
			},
		},
	}, metadataResponse)

	// Test Payloads
	unsignedRaw := `{"from":"0xD10a72Cf054650931365Cc44D912a4FD75257058","to":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d","value":"0x9864aac3510d02","data":"0x","nonce":"0x1","gas_price":"0x3b9aca00","gas":"0x5208","chain_id":"0x13881"}` // nolint
	payloadsResponse, err := servicer.ConstructionPayloads(ctx, &types.ConstructionPayloadsRequest{
		NetworkIdentifier: networkIdentifier,
		Operations:        ops,
		Metadata:          forceMarshalMap(t, metadata),
	})
	assert.Nil(t, err)
	payloadsRaw := `[{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058","hex_bytes":"9fc67756448ac9767dd028418bc7970d843ffe283ea7b2a96e33392c3e13d3a8","account_identifier":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"signature_type":"ecdsa_recovery"}]` // nolint
	var payloads []*types.SigningPayload
	assert.NoError(t, json.Unmarshal([]byte(payloadsRaw), &payloads))
	assert.Equal(t, &types.ConstructionPayloadsResponse{
		UnsignedTransaction: unsignedRaw,
		Payloads:            payloads,
	}, payloadsResponse)

	// Test Parse Unsigned
	parseOpsRaw := `[{"operation_identifier":{"index":0},"type":"CALL","account":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"amount":{"value":"-42894881044106498","currency":{"symbol":"MATIC","decimals":18}}},{"operation_identifier":{"index":1},"related_operations":[{"index":0}],"type":"CALL","account":{"address":"0x57B414a0332B5CaB885a451c2a28a07d1e9b8a8d"},"amount":{"value":"42894881044106498","currency":{"symbol":"MATIC","decimals":18}}}]` // nolint
	var parseOps []*types.Operation
	assert.NoError(t, json.Unmarshal([]byte(parseOpsRaw), &parseOps))
	parseUnsignedResponse, err := servicer.ConstructionParse(ctx, &types.ConstructionParseRequest{
		NetworkIdentifier: networkIdentifier,
		Signed:            false,
		Transaction:       unsignedRaw,
	})
	assert.Nil(t, err)
	parseMetadata := &parseMetadata{
		Nonce:    metadata.Nonce,
		GasPrice: metadata.GasPrice,
		GasLimit: metadata.GasLimit.Uint64(),
		ChainID:  big.NewInt(80001),
	}
	assert.Equal(t, &types.ConstructionParseResponse{
		Operations:               parseOps,
		AccountIdentifierSigners: []*types.AccountIdentifier{},
		Metadata:                 forceMarshalMap(t, parseMetadata),
	}, parseUnsignedResponse)

	// Test Combine
	signaturesRaw := `[{"hex_bytes":"f41bbaff27975ce07e5ab216c33f02384ef79290ef83537c452a61827d80d51a73e5a4707d46f03f4a5841de0623b744942e9edf6e0ef3761acdc40c7f147dc301","signing_payload":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058","hex_bytes":"9fc67756448ac9767dd028418bc7970d843ffe283ea7b2a96e33392c3e13d3a8","account_identifier":{"address":"0xD10a72Cf054650931365Cc44D912a4FD75257058"},"signature_type":"ecdsa_recovery"},"public_key":{"hex_bytes":"0212e9f98d9750e5f74b4b4b00df39074f86c79187943bdb3c5a9c89ffc1ed0188","curve_type":"secp256k1"},"signature_type":"ecdsa_recovery"}]` // nolint
	var signatures []*types.Signature
	assert.NoError(t, json.Unmarshal([]byte(signaturesRaw), &signatures))
	signedRaw := `{"nonce":"0x1","gasPrice":"0x3b9aca00","gas":"0x5208","to":"0x57b414a0332b5cab885a451c2a28a07d1e9b8a8d","value":"0x9864aac3510d02","input":"0x","v":"0x27126","r":"0xf41bbaff27975ce07e5ab216c33f02384ef79290ef83537c452a61827d80d51a","s":"0x73e5a4707d46f03f4a5841de0623b744942e9edf6e0ef3761acdc40c7f147dc3","hash":"0x06c3e9f1e4d6309b33e86aeb91b0b64d58057246d5d1fb4302bbb3fe2c745b3c"}` // nolint
	combineResponse, err := servicer.ConstructionCombine(ctx, &types.ConstructionCombineRequest{
		NetworkIdentifier:   networkIdentifier,
		UnsignedTransaction: unsignedRaw,
		Signatures:          signatures,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionCombineResponse{
		SignedTransaction: signedRaw,
	}, combineResponse)

	// Test Parse Signed
	parseSignedResponse, err := servicer.ConstructionParse(ctx, &types.ConstructionParseRequest{
		NetworkIdentifier: networkIdentifier,
		Signed:            true,
		Transaction:       signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.ConstructionParseResponse{
		Operations: parseOps,
		AccountIdentifierSigners: []*types.AccountIdentifier{
			{Address: "0xD10a72Cf054650931365Cc44D912a4FD75257058"},
		},
		Metadata: forceMarshalMap(t, parseMetadata),
	}, parseSignedResponse)

	// Test Hash
	transactionIdentifier := &types.TransactionIdentifier{
		Hash: "0x06c3e9f1e4d6309b33e86aeb91b0b64d58057246d5d1fb4302bbb3fe2c745b3c",
	}
	hashResponse, err := servicer.ConstructionHash(ctx, &types.ConstructionHashRequest{
		NetworkIdentifier: networkIdentifier,
		SignedTransaction: signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.TransactionIdentifierResponse{
		TransactionIdentifier: transactionIdentifier,
	}, hashResponse)

	// Test Submit
	mockClient.On(
		"SendTransaction",
		ctx,
		mock.Anything, // can't test ethTx here because it contains "time"
	).Return(
		nil,
	)
	submitResponse, err := servicer.ConstructionSubmit(ctx, &types.ConstructionSubmitRequest{
		NetworkIdentifier: networkIdentifier,
		SignedTransaction: signedRaw,
	})
	assert.Nil(t, err)
	assert.Equal(t, &types.TransactionIdentifierResponse{
		TransactionIdentifier: transactionIdentifier,
	}, submitResponse)

	mockClient.AssertExpectations(t)
}

func templateError(error *types.Error, context string) *types.Error {
	return &types.Error{
		Code:      error.Code,
		Message:   error.Message,
		Retriable: false,
		Details: map[string]interface{}{
			"context": context,
		},
	}
}

func templateOperations(amount uint64, currency *types.Currency) []*types.Operation {
	return rosettaOperations(
		fromAddress,
		toAddress,
		big.NewInt(int64(amount)),
		currency,
	)
}
